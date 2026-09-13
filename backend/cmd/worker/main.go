package main

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"fihos/backend/internal/config"
	"fihos/backend/internal/database"
	"fihos/backend/internal/mikrotik"
	"fihos/backend/internal/service"
	"fihos/backend/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	defer rdb.Close()

	st := store.New(db)
	sim := mikrotik.NewSimulator()
	mt := mikrotik.NewClient(sim)
	anom := service.NewAnomalyEngine(st)

	log.Printf("worker started (sync=%s, probe=%s, analytics=%s)", cfg.RouterSyncFreq, cfg.RouterProbeFreq, cfg.AnalyticsFreq)
	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	// Initial probe so scheduler doesn't fail on first tick.
	probeRouters(ctx, st, sim, mt)

	var wg sync.WaitGroup
	wg.Add(3)

	// Probe loop: register + probe routers (immediate first pass).
	go func() {
		defer wg.Done()
		for {
			probeRouters(syncCtx, st, sim, mt)
			select {
			case <-syncCtx.Done():
				return
			case <-time.After(cfg.RouterProbeFreq):
			}
		}
	}()

	// Scheduler loop: config jobs, rate windows, session reconciliation.
	go func() {
		defer wg.Done()
		for {
			schedulerTick(syncCtx, st, mt, cfg.RouterSyncFreq)
			select {
			case <-syncCtx.Done():
				return
			case <-time.After(cfg.RouterSyncFreq):
			}
		}
	}()

	// Analytics loop: daily rollups + anomaly engine (delayed first pass).
	go func() {
		defer wg.Done()
		select {
		case <-syncCtx.Done():
			return
		case <-time.After(cfg.AnalyticsFreq):
		}
		for {
			analyticsTick(syncCtx, st, anom)
			select {
			case <-syncCtx.Done():
				return
			case <-time.After(cfg.AnalyticsFreq):
			}
		}
	}()

	<-syncCtx.Done()
	cancel()
	wg.Wait()
	log.Println("worker stopped")
}

func probeRouters(ctx context.Context, st *store.Store, sim *mikrotik.Simulator, mt *mikrotik.Client) {
	if ctx.Err() != nil {
		return
	}
	routers, err := st.RoutersAll(ctx)
	if err != nil {
		log.Printf("probe list: %v", err)
		return
	}
	for _, r := range routers {
		sim.RegisterRouter(r.ID, r.Name)
		ok, ts, err := mt.ProbeWithTime(mikrotik.Router{ID: r.ID})
		if err != nil || !ok {
			_ = st.SetRouterOffline(ctx, r.ID)
			continue
		}
		_ = st.SetRouterOnline(ctx, r.ID)
		_ = st.SetRouterLastSync(ctx, r.ID, ts)
	}
}

func schedulerTick(ctx context.Context, st *store.Store, mt *mikrotik.Client, interval time.Duration) {
	if ctx.Err() != nil {
		return
	}

	// 1. Apply pending hotspot config jobs.
	jobs, err := st.PendingJobs(ctx, 20)
	if err != nil {
		log.Printf("jobs: %v", err)
		return
	}
	for _, j := range jobs {
		hs, err := st.HotspotByID(ctx, j.TenantID, j.HotspotID)
		if err != nil {
			log.Printf("job %d hotspot: %v", j.ID, err)
			continue
		}
		profile, err := st.ProfileByID(ctx, j.TenantID, hs.ProfileID)
		if err != nil {
			_ = st.FailJob(ctx, j.ID, err.Error())
			continue
		}
		if err := mt.ApplyConfig(mikrotik.Router{ID: hs.RouterID},
			mikrotik.HotspotConfig{Name: hs.Name, RxRate: profile.RxRate, TxRate: profile.TxRate, UptimeLimit: profile.SessionUptimeLimit}); err != nil {
			_ = st.FailJob(ctx, j.ID, err.Error())
			continue
		}
		if err := st.DoneJob(ctx, j.ID); err != nil {
			log.Printf("job %d done: %v", j.ID, err)
		}
		if err := st.SetHotspotConfigured(ctx, hs.ID, "", hs.AppliedMultiplier); err != nil {
			log.Printf("hotspot set configured %d: %v", hs.ID, err)
		}
		log.Printf("config applied: hotspot=%s", hs.Name)
	}

	// 2. Start rate windows that became effective.
	windows, err := st.WindowsToStart(ctx)
	if err != nil {
		log.Printf("rate-window start: %v", err)
		return
	}
	for _, w := range windows {
		if err := mt.ApplyRateMultiplier(mikrotik.Router{ID: w.RouterID}, w.HotspotName, w.Multiplier); err != nil {
			log.Printf("rate-window start h=%d: %v", w.HotspotID, err)
			continue
		}
		_ = st.SetHotspotConfigured(ctx, w.HotspotID, "", w.Multiplier)
		log.Printf("rate window applied: hotspot=%s x%.1f", w.HotspotName, w.Multiplier)
	}

	// 3. Revert expired rate windows.
	ends, err := st.ListExpiredAppliedWindows(ctx, time.Now())
	if err != nil {
		log.Printf("rate-window end: %v", err)
		return
	}
	for _, e := range ends {
		if err := mt.ApplyRateMultiplier(mikrotik.Router{ID: e.RouterID}, e.HotspotName, 1); err != nil {
			log.Printf("rate-window end h=%d: %v", e.HotspotID, err)
			continue
		}
		_ = st.SetHotspotConfigured(ctx, e.HotspotID, "", 1)
		log.Printf("rate window reverted: hotspot=%s", e.HotspotName)
	}

	// 4. Reconcile live sessions (mirror DB -> sim, poll traffic, snapshot).
	for _, r := range routersWithActiveSessions(ctx, st) {
		reconcileRouter(ctx, st, mt, r, interval)
	}
}

type routerSessions struct {
	routerID int64
	sessions []store.Session
}

func routersWithActiveSessions(ctx context.Context, st *store.Store) []routerSessions {
	sessions, err := st.ListActiveSessions(ctx, 500, 0)
	if err != nil {
		log.Printf("active sessions: %v", err)
		return nil
	}
	grouped := map[int64][]store.Session{}
	for _, s := range sessions {
		grouped[s.RouterID] = append(grouped[s.RouterID], s)
	}
	out := make([]routerSessions, 0, len(grouped))
	for rid, ss := range grouped {
		out = append(out, routerSessions{routerID: rid, sessions: ss})
	}
	return out
}

func reconcileRouter(ctx context.Context, st *store.Store, mt *mikrotik.Client, rs routerSessions, interval time.Duration) {
	// Mirror current DB sessions into the simulated router.
	seed := make([]mikrotik.SimSession, 0, len(rs.sessions))
	keep := map[string]bool{}
	for i := range rs.sessions {
		s := rs.sessions[i]
		seed = append(seed, mikrotik.SimSession{
			Username: s.Username,
			MAC:      derefStr(s.MacAddress),
			IP:       derefStr(s.IPAddress),
			BytesRX:  s.BytesRX,
			BytesTX:  s.BytesTX,
		})
		keep[s.Username] = true
	}
	router := mikrotik.Router{ID: rs.routerID}
	mt.SeedSessions(router, seed)
	mt.DropSessionsExcept(router, keep)

	live, err := mt.ListSessions(router, interval.Seconds())
	if err != nil {
		log.Printf("list sessions r=%d: %v", rs.routerID, err)
		return
	}
	byUser := map[string]mikrotik.SimSession{}
	for _, s := range live {
		byUser[s.Username] = s
	}

	now := time.Now().UTC()
	for i := range rs.sessions {
		s := rs.sessions[i]
		if lv, ok := byUser[s.Username]; ok {
			if _, err := st.SnapshotSessionUsage(ctx, s.ID, now, lv.BytesRX, lv.BytesTX); err != nil {
				log.Printf("snapshot session %d: %v", s.ID, err)
			}
			continue
		}
		// Session disappeared from the simulated router: close it.
		if _, err := st.CloseSession(ctx, s.ID, now, s.BytesRX, s.BytesTX); err != nil {
			log.Printf("close session %d: %v", s.ID, err)
		}
	}
}

func analyticsTick(ctx context.Context, st *store.Store, anom *service.AnomalyEngine) {
	if ctx.Err() != nil {
		return
	}
	tenants, err := st.ActiveTenantIDs(ctx)
	if err != nil {
		log.Printf("tenants: %v", err)
		return
	}
	for _, tid := range tenants {
		if err := st.RollupDailies(ctx, tid, 30); err != nil {
			log.Printf("rollup tenant %d: %v", tid, err)
		}
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
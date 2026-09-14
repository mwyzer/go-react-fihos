package mikrotik

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) redis.Cmdable {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestRedisSimulatorSharesStateAcrossInstances(t *testing.T) {
	rdb := newTestRedis(t)
	api := NewRedisSimulator(rdb)
	worker := NewRedisSimulator(rdb)

	api.RegisterRouter(7, "RB750")
	ok, ts, err := worker.Probe(7)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !ok {
		t.Fatal("expected router online after register from other instance")
	}
	if ts.IsZero() {
		t.Fatal("expected last_seen set")
	}

	// Offline toggle from one side is observed by the other.
	api.Reset(7)
	ok, _, err = worker.Probe(7)
	if err != nil {
		t.Fatalf("probe after reset: %v", err)
	}
	if ok {
		t.Fatal("expected router offline after reset from other instance")
	}
}

func TestRedisSimulatorUnknownRouter(t *testing.T) {
	rdb := newTestRedis(t)
	sim := NewRedisSimulator(rdb)
	if _, _, err := sim.Probe(99); err != ErrUnknownRouter {
		t.Fatalf("expected ErrUnknownRouter, got %v", err)
	}
}

func TestRedisSimulatorLifecycle(t *testing.T) {
	rdb := newTestRedis(t)
	sim := NewRedisSimulator(rdb)

	sim.RegisterRouter(1, "edge")
	if err := sim.ApplyHotspotConfig(1, "Kopi", 4096, 2048, 3600); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	if err := sim.ApplyRateMultiplier(1, "Kopi", 2); err != nil {
		t.Fatalf("apply multiplier: %v", err)
	}
	if err := sim.ApplyRateMultiplier(1, "Missing", 2); err != ErrNoHotspot {
		t.Fatalf("expected ErrNoHotspot, got %v", err)
	}

	sim.SeedSession(1, "v-0001", "AA:BB", "10.0.0.5", 10000, 2000)
	time.Sleep(2 * time.Millisecond)
	sessions, err := sim.ListSessions(1, 30)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].BytesRX <= 10000 {
		t.Fatalf("expected counters to advance, got %d", sessions[0].BytesRX)
	}

	if err := sim.DisconnectSession(1, "v-0001"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	sessions, err = sim.ListSessions(1, 1)
	if err != nil {
		t.Fatalf("list after disconnect: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}

	sim.UnregisterRouter(1)
	if err := sim.ApplyHotspotConfig(1, "Kopi", 1, 1, 1); err != ErrUnknownRouter {
		t.Fatalf("expected ErrUnknownRouter after unregister, got %v", err)
	}
}

func TestRedisSimulatorOfflineRejectsWrites(t *testing.T) {
	rdb := newTestRedis(t)
	sim := NewRedisSimulator(rdb)

	sim.RegisterRouter(3, "rb")
	sim.Reset(3)
	if err := sim.ApplyHotspotConfig(3, "H", 1, 1, 1); err != ErrRouterOffline {
		t.Fatalf("expected ErrRouterOffline, got %v", err)
	}
	if _, err := sim.ListSessions(3, 1); err != ErrRouterOffline {
		t.Fatalf("expected ErrRouterOffline, got %v", err)
	}
}
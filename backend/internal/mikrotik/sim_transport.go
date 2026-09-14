package mikrotik

import (
	"time"
)

// simTransport implements Transport over a SimState (in-memory or shared via
// Redis), preserving the behavior the platform shipped with.
type simTransport struct {
	sim SimState
}

func newSimTransport(sim SimState) *simTransport {
	return &simTransport{sim: sim}
}

func (t *simTransport) Register(router Router) {
	t.sim.RegisterRouter(router.ID, router.Name)
}

func (t *simTransport) Unregister(routerID int64) {
	t.sim.UnregisterRouter(routerID)
}

func (t *simTransport) Probe(router Router) (bool, time.Time, error) {
	return t.sim.Probe(router.ID)
}

func (t *simTransport) ApplyConfig(router Router, cfg HotspotConfig) error {
	return t.sim.ApplyHotspotConfig(router.ID, cfg.Name, cfg.RxRate, cfg.TxRate, cfg.UptimeLimit)
}

func (t *simTransport) ApplyRateMultiplier(router Router, hotspotName string, multiplier float64) error {
	return t.sim.ApplyRateMultiplier(router.ID, hotspotName, multiplier)
}

func (t *simTransport) SeedSessions(router Router, sessions []SimSession) {
	for _, s := range sessions {
		t.sim.SeedSession(router.ID, s.Username, s.MAC, s.IP, s.BytesRX, s.BytesTX)
	}
}

// DropSessionsExcept is a no-op: the simulation keeps sessions until a router
// restarts or the API disconnects them explicitly.
func (t *simTransport) DropSessionsExcept(router Router, keep map[string]bool) {
	_ = router
	_ = keep
}

func (t *simTransport) ListSessions(router Router, intervalSec float64) ([]SimSession, error) {
	return t.sim.ListSessions(router.ID, intervalSec)
}

func (t *simTransport) Disconnect(router Router, username string) error {
	return t.sim.DisconnectSession(router.ID, username)
}
package mikrotik

import "time"

// Router identifies a device the client talks to.
type Router struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	APIPort   int    `json:"api_port"`
	Username  string `json:"username"`
	Password  string `json:"-"`
}

// HotspotConfig is the profile applied to a hotspot inside RouterOS.
type HotspotConfig struct {
	Name        string `json:"name"`
	RxRate      int64  `json:"rx_rate"`
	TxRate      int64  `json:"tx_rate"`
	UptimeLimit int    `json:"session_uptime_limit"`
}

// Client talks to simulated (or later real) RouterOS devices.
// The simulator is injected so api and worker share one in-memory registry.
type Client struct {
	sim *Simulator
}

func NewClient(sim *Simulator) *Client {
	return &Client{sim: sim}
}

func (c *Client) Register(router Router) {
	c.sim.RegisterRouter(router.ID, router.Name)
}

func (c *Client) Unregister(routerID int64) {
	c.sim.UnregisterRouter(routerID)
}

func (c *Client) Probe(router Router) (bool, error) {
	ok, _, err := c.sim.Probe(router.ID)
	return ok, err
}

func (c *Client) ProbeWithTime(router Router) (bool, time.Time, error) {
	return c.sim.Probe(router.ID)
}

func (c *Client) ApplyConfig(router Router, cfg HotspotConfig) error {
	return c.sim.ApplyHotspotConfig(router.ID, cfg.Name, cfg.RxRate, cfg.TxRate, cfg.UptimeLimit)
}

// ApplyRateMultiplier boosts rates for an active rate window. Config pushes
// are idempotent.
func (c *Client) ApplyRateMultiplier(router Router, hotspotName string, multiplier float64) error {
	return c.sim.ApplyRateMultiplier(router.ID, hotspotName, multiplier)
}

func (c *Client) SeedSessions(router Router, sessions []SimSession) {
	for _, s := range sessions {
		c.sim.SeedSession(router.ID, s.Username, s.MAC, s.IP, s.BytesRX, s.BytesTX)
	}
}

func (c *Client) DropSessionsExcept(router Router, keep map[string]bool) {
	// no-op: simulation keeps sessions until router restarts or API disconnects
	_ = router
	_ = keep
}

func (c *Client) ListSessions(router Router, intervalSec float64) ([]SimSession, error) {
	return c.sim.ListSessions(router.ID, intervalSec)
}

func (c *Client) Disconnect(router Router, username string) error {
	return c.sim.DisconnectSession(router.ID, username)
}
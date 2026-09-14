package mikrotik

import (
	"net/http"
	"time"
)

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

// Transport abstracts a RouterOS data path. The simulator backend keeps the
// platform fully exercisable without hardware; the REST backend talks to real
// RouterOS 6.47+/7.x devices over its JSON-RPC REST API ((/rest/...)).
type Transport interface {
	Register(router Router)
	Unregister(routerID int64)
	Probe(router Router) (bool, time.Time, error)
	ApplyConfig(router Router, cfg HotspotConfig) error
	ApplyRateMultiplier(router Router, hotspotName string, multiplier float64) error
	SeedSessions(router Router, sessions []SimSession)
	DropSessionsExcept(router Router, keep map[string]bool)
	ListSessions(router Router, intervalSec float64) ([]SimSession, error)
	Disconnect(router Router, username string) error
}

// Mode selects the transport backend.
const (
	ModeSimulate = "simulate"
	ModeREST     = "rest"
)

// New builds a Client backed by the chosen transport. simulate keeps the
// device emulator (default); rest talks to real devices using the supplied
// credentials/HTTP client.
func New(mode string, sim SimState, creds Credentials, httpc *http.Client) *Client {
	if mode == ModeREST {
		return &Client{tr: newRestTransport(creds, httpc)}
	}
	return &Client{tr: newSimTransport(sim)}
}

// Client is the frontend handed to handlers and the worker. It forwards every
// call to the configured Transport so callers never change with the backend.
type Client struct {
	tr Transport
}

// NewClient retains the legacy simulator-only construction.
func NewClient(sim *Simulator) *Client {
	return New(ModeSimulate, sim, Credentials{}, nil)
}

func (c *Client) Register(router Router)          { c.tr.Register(router) }
func (c *Client) Unregister(routerID int64)       { c.tr.Unregister(routerID) }
func (c *Client) Probe(router Router) (bool, error) {
	ok, _, err := c.tr.Probe(router)
	return ok, err
}
func (c *Client) ProbeWithTime(router Router) (bool, time.Time, error) {
	return c.tr.Probe(router)
}
func (c *Client) ApplyConfig(router Router, cfg HotspotConfig) error {
	return c.tr.ApplyConfig(router, cfg)
}
func (c *Client) ApplyRateMultiplier(router Router, hotspotName string, multiplier float64) error {
	return c.tr.ApplyRateMultiplier(router, hotspotName, multiplier)
}
func (c *Client) SeedSessions(router Router, sessions []SimSession) {
	c.tr.SeedSessions(router, sessions)
}
func (c *Client) DropSessionsExcept(router Router, keep map[string]bool) {
	c.tr.DropSessionsExcept(router, keep)
}
func (c *Client) ListSessions(router Router, intervalSec float64) ([]SimSession, error) {
	return c.tr.ListSessions(router, intervalSec)
}
func (c *Client) Disconnect(router Router, username string) error {
	return c.tr.Disconnect(router, username)
}
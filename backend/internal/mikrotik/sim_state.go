package mikrotik

import (
	"time"
)

// SimState is the device-state surface used by simTransport and by API/worker
// wiring. The in-memory *Simulator and the Redis-backed *RedisSimulator both
// implement it, so a multi-process deployment sees one consistent view of the
// emulated devices instead of one private copy per process.
type SimState interface {
	RegisterRouter(id int64, name string)
	UnregisterRouter(id int64)
	Reset(id int64)
	Probe(id int64) (bool, time.Time, error)
	ApplyHotspotConfig(id int64, hotspotName string, rx, tx int64, uptimeLimit int) error
	ApplyRateMultiplier(id int64, hotspotName string, multiplier float64) error
	SeedSession(id int64, username, mac, ip string, bytesRX, bytesTX int64)
	DisconnectSession(id int64, username string) error
	ListSessions(id int64, intervalSec float64) ([]SimSession, error)
}

var _ SimState = (*Simulator)(nil)
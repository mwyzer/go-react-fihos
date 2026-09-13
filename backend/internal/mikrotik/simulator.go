package mikrotik

import (
	"math/rand"
	"sync"
	"time"
)

type SimSession struct {
	Username string `json:"username"`
	MAC      string `json:"mac_address"`
	IP       string `json:"ip_address"`
	BytesRX  int64  `json:"bytes_rx"`
	BytesTX  int64  `json:"bytes_tx"`
}

type SimHotspot struct {
	Name        string
	BaseRx      int64
	BaseTx      int64
	RxRate      int64
	TxRate      int64
	Multiplier  float64
	UptimeLimit int
}

type simRouter struct {
	ID        int64
	Name      string
	Connected bool
	LastSeen  time.Time
	Sessions  map[string]*SimSession
	Hotspots  map[string]*SimHotspot
}

// Simulator is a threadsafe, in-memory emulation of RouterOS devices so the
// whole platform can be exercised without real hardware.
type Simulator struct {
	mu      sync.RWMutex
	routers map[int64]*simRouter
	now     func() time.Time
	rand    *rand.Rand
}

func NewSimulator() *Simulator {
	return &Simulator{
		routers: make(map[int64]*simRouter),
		now:     time.Now,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Simulator) RegisterRouter(id int64, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.routers[id]
	if !ok {
		r = &simRouter{ID: id, Name: name, Sessions: map[string]*SimSession{}, Hotspots: map[string]*SimHotspot{}}
		s.routers[id] = r
	}
	r.Connected = true
	r.LastSeen = s.now()
}

func (s *Simulator) UnregisterRouter(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.routers, id)
}

// Reset makes a router offline (simulating power loss or network partition).
func (s *Simulator) Reset(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.routers[id]; ok {
		r.Connected = false
	}
}

// SetNow overrides the clock source for deterministic tests.
func (s *Simulator) SetNow(fn func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = fn
}

func (s *Simulator) router(id int64) (*simRouter, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.routers[id]
	return r, ok
}

func (s *Simulator) Probe(id int64) (bool, time.Time, error) {
	r, ok := s.router(id)
	if !ok {
		return false, time.Time{}, ErrUnknownRouter
	}
	s.mu.Lock()
	r.LastSeen = s.now()
	s.mu.Unlock()
	return r.Connected, r.LastSeen, nil
}

func (s *Simulator) ApplyHotspotConfig(id int64, hotspotName string, rx, tx int64, uptimeLimit int) error {
	r, ok := s.router(id)
	if !ok {
		return ErrUnknownRouter
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !r.Connected {
		return ErrRouterOffline
	}
	h, ok := r.Hotspots[hotspotName]
	if !ok {
		h = &SimHotspot{Name: hotspotName, Multiplier: 1}
		r.Hotspots[hotspotName] = h
	}
	h.BaseRx = rx
	h.BaseTx = tx
	h.RxRate = int64(float64(rx) * h.Multiplier)
	h.TxRate = int64(float64(tx) * h.Multiplier)
	h.UptimeLimit = uptimeLimit
	return nil
}

func (s *Simulator) ApplyRateMultiplier(id int64, hotspotName string, multiplier float64) error {
	r, ok := s.router(id)
	if !ok {
		return ErrUnknownRouter
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !r.Connected {
		return ErrRouterOffline
	}
	h, ok := r.Hotspots[hotspotName]
	if !ok {
		return ErrNoHotspot
	}
	h.Multiplier = multiplier
	h.RxRate = int64(float64(h.BaseRx) * multiplier)
	h.TxRate = int64(float64(h.BaseTx) * multiplier)
	return nil
}

func (s *Simulator) SeedSession(id int64, username, mac, ip string, bytesRX, bytesTX int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.routers[id]; ok && r.Connected {
		r.Sessions[username] = &SimSession{Username: username, MAC: mac, IP: ip, BytesRX: bytesRX, BytesTX: bytesTX}
	}
}

func (s *Simulator) DropSession(id int64, username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.routers[id]; ok {
		delete(r.Sessions, username)
	}
}

func (s *Simulator) DisconnectSession(id int64, username string) error {
	r, ok := s.router(id)
	if !ok {
		return ErrUnknownRouter
	}
	if !r.Connected {
		return ErrRouterOffline
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(r.Sessions, username)
	return nil
}

// ListSessions returns live sessions with traffic counters advanced by the
// elapsed time since the last poll, scaled by the hotspot's applied rates.
func (s *Simulator) ListSessions(id int64, intervalSec float64) ([]SimSession, error) {
	r, ok := s.router(id)
	if !ok {
		return nil, ErrUnknownRouter
	}
	if !r.Connected {
		return nil, ErrRouterOffline
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]SimSession, 0, len(r.Sessions))
	for _, sess := range r.Sessions {
		jit := 0.6 + s.rand.Float64()*0.8
		rate := s.rateForSession(r, sess)
		sess.BytesRX += int64(rate * intervalSec * 125 * jit)
		sess.BytesTX += int64(rate * intervalSec * 125 * jit * 0.28)
		out = append(out, *sess)
		if s.rand.Float64() < 0.03 && len(r.Sessions) > 1 {
			delete(r.Sessions, sess.Username)
		}
	}
	return out, nil
}

func (s *Simulator) rateForSession(r *simRouter, sess *SimSession) float64 {
	max := float64(0)
	mult := 1.0
	for _, h := range r.Hotspots {
		m := float64(h.RxRate)
		if m > max {
			max = m
			mult = h.Multiplier
		}
	}
	_ = mult
	if max <= 0 {
		max = 1024
	}
	return max
}

var (
	ErrUnknownRouter = errSim("unknown router")
	ErrRouterOffline = errSim("router offline")
	ErrNoHotspot     = errSim("hotspot not configured")
)

type errSim string

func (e errSim) Error() string { return string(e) }
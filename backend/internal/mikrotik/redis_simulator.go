package mikrotik

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSimulator is a SimState backed by Redis so the API and worker processes
// (which each run their own HTTP/services stack) agree on the emulated RouterOS
// state instead of keeping private in-memory copies. Sessions, hotspots and
// online/offline flags are stored as one JSON document per router.
type RedisSimulator struct {
	rdb redis.Cmdable
	key string
}

const redisSimPrefix = "fihos:sim:router:"

// NewRedisSimulator builds a simulator that persists device state under
// keyPrefix (default "fihos:sim:router:" when empty).
func NewRedisSimulator(rdb redis.Cmdable) *RedisSimulator {
	return &RedisSimulator{rdb: rdb, key: redisSimPrefix}
}

var _ SimState = (*RedisSimulator)(nil)

func (s *RedisSimulator) keyFor(id int64) string {
	return fmt.Sprintf("%s%d", s.key, id)
}

func (s *RedisSimulator) getRouter(ctx context.Context, id int64) (*simRouter, error) {
	data, err := s.rdb.Get(ctx, s.keyFor(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrUnknownRouter
	}
	if err != nil {
		return nil, err
	}
	var r simRouter
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *RedisSimulator) putRouter(ctx context.Context, r *simRouter) error {
	if r.Sessions == nil {
		r.Sessions = map[string]*SimSession{}
	}
	if r.Hotspots == nil {
		r.Hotspots = map[string]*SimHotspot{}
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.keyFor(r.ID), data, 0).Err()
}

func (s *RedisSimulator) RegisterRouter(id int64, name string) {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if errors.Is(err, ErrUnknownRouter) {
		r = &simRouter{ID: id, Name: name, Sessions: map[string]*SimSession{}, Hotspots: map[string]*SimHotspot{}}
	} else if err != nil {
		return
	}
	r.Connected = true
	r.LastSeen = time.Now()
	if name != "" {
		r.Name = name
	}
	_ = s.putRouter(ctx, r)
}

func (s *RedisSimulator) UnregisterRouter(id int64) {
	_ = s.rdb.Del(context.Background(), s.keyFor(id)).Err()
}

func (s *RedisSimulator) Reset(id int64) {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if err != nil {
		return
	}
	r.Connected = false
	_ = s.putRouter(ctx, r)
}

func (s *RedisSimulator) Probe(id int64) (bool, time.Time, error) {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if err != nil {
		return false, time.Time{}, err
	}
	r.LastSeen = time.Now()
	_ = s.putRouter(ctx, r)
	return r.Connected, r.LastSeen, nil
}

func (s *RedisSimulator) ApplyHotspotConfig(id int64, hotspotName string, rx, tx int64, uptimeLimit int) error {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if err != nil {
		return err
	}
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
	return s.putRouter(ctx, r)
}

func (s *RedisSimulator) ApplyRateMultiplier(id int64, hotspotName string, multiplier float64) error {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if err != nil {
		return err
	}
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
	return s.putRouter(ctx, r)
}

func (s *RedisSimulator) SeedSession(id int64, username, mac, ip string, bytesRX, bytesTX int64) {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if err != nil || !r.Connected {
		return
	}
	r.Sessions[username] = &SimSession{Username: username, MAC: mac, IP: ip, BytesRX: bytesRX, BytesTX: bytesTX}
	_ = s.putRouter(ctx, r)
}

func (s *RedisSimulator) DisconnectSession(id int64, username string) error {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if err != nil {
		return err
	}
	if !r.Connected {
		return ErrRouterOffline
	}
	delete(r.Sessions, username)
	return s.putRouter(ctx, r)
}

// ListSessions returns live sessions with traffic counters advanced by the
// elapsed time since the last poll, scaled by the hotspot's applied rates
// (mirrors the in-memory Simulator semantics).
func (s *RedisSimulator) ListSessions(id int64, intervalSec float64) ([]SimSession, error) {
	ctx := context.Background()
	r, err := s.getRouter(ctx, id)
	if err != nil {
		return nil, err
	}
	if !r.Connected {
		return nil, ErrRouterOffline
	}

	out := make([]SimSession, 0, len(r.Sessions))
	names := make([]string, 0, len(r.Sessions))
	for name := range r.Sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sess := r.Sessions[name]
		jit := 0.6 + rand.Float64()*0.8
		rate := s.rateForRouter(r)
		sess.BytesRX += int64(rate * intervalSec * 125 * jit)
		sess.BytesTX += int64(rate * intervalSec * 125 * jit * 0.28)
		out = append(out, *sess)
		if rand.Float64() < 0.03 && len(r.Sessions) > 1 {
			delete(r.Sessions, name)
		}
	}
	return out, s.putRouter(ctx, r)
}

func (s *RedisSimulator) rateForRouter(r *simRouter) float64 {
	max := float64(0)
	for _, h := range r.Hotspots {
		if m := float64(h.RxRate); m > max {
			max = m
		}
	}
	if max <= 0 {
		return 1024
	}
	return max
}
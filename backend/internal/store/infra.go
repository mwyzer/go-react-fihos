package store

import (
	"context"
	"time"
)

type Profile struct {
	ID                 int64     `json:"id"`
	TenantID           int64     `json:"tenant_id"`
	Name               string    `json:"name"`
	RxRate             int64     `json:"rx_rate"`
	TxRate             int64     `json:"tx_rate"`
	SessionUptimeLimit int       `json:"session_uptime_limit"`
	KeepaliveTimeout   int       `json:"keepalive_timeout"`
	CreatedBy          *int64    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
}

type Router struct {
	ID         int64      `json:"id"`
	TenantID   int64      `json:"tenant_id"`
	Name       string     `json:"name"`
	IPAddress  string     `json:"ip_address"`
	APIPort    int        `json:"api_port"`
	Username   string     `json:"username"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Hotspot struct {
	ID                int64      `json:"id"`
	TenantID          int64      `json:"tenant_id"`
	RouterID          int64      `json:"router_id"`
	ProfileID         int64      `json:"profile_id"`
	Name              string     `json:"name"`
	MikrotikID        *string    `json:"mikrotik_id"`
	IPRange           *string    `json:"ip_range"`
	Status            string     `json:"status"`
	LastConfigAt      *time.Time `json:"last_config_at"`
	AppliedMultiplier float64    `json:"applied_multiplier"`
	CreatedAt         time.Time  `json:"created_at"`
}

type ConfigJob struct {
	ID        int64          `json:"id"`
	TenantID  int64          `json:"tenant_id"`
	HotspotID int64          `json:"hotspot_id"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload"`
	Status    string         `json:"status"`
	Attempts  int            `json:"attempts"`
	LastError *string        `json:"last_error"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

const profileCols = `id, tenant_id, name, rx_rate, tx_rate, session_uptime_limit, keepalive_timeout, created_by, created_at`
const routerCols = `id, tenant_id, name, ip_address::text, api_port, username, status, last_seen_at, last_sync_at, created_at`
const hotspotCols = `id, tenant_id, router_id, profile_id, name, mikrotik_id, ip_range::text, status, last_config_at, applied_multiplier::float8, created_at`
const jobCols = `id, tenant_id, hotspot_id, action, payload, status, attempts, last_error, created_at, updated_at`

func scanProfile(row interface{ Scan(...any) error }) (*Profile, error) {
	p := &Profile{}
	err := row.Scan(&p.ID, &p.TenantID, &p.Name, &p.RxRate, &p.TxRate, &p.SessionUptimeLimit, &p.KeepaliveTimeout, &p.CreatedBy, &p.CreatedAt)
	return p, err
}

func scanRouter(row interface{ Scan(...any) error }) (*Router, error) {
	r := &Router{}
	err := row.Scan(&r.ID, &r.TenantID, &r.Name, &r.IPAddress, &r.APIPort, &r.Username, &r.Status, &r.LastSeenAt, &r.LastSyncAt, &r.CreatedAt)
	return r, err
}

func scanHotspot(row interface{ Scan(...any) error }) (*Hotspot, error) {
	h := &Hotspot{}
	err := row.Scan(&h.ID, &h.TenantID, &h.RouterID, &h.ProfileID, &h.Name, &h.MikrotikID, &h.IPRange, &h.Status, &h.LastConfigAt, &h.AppliedMultiplier, &h.CreatedAt)
	return h, err
}

func scanJob(row interface{ Scan(...any) error }) (*ConfigJob, error) {
	j := &ConfigJob{}
	err := row.Scan(&j.ID, &j.TenantID, &j.HotspotID, &j.Action, &j.Payload, &j.Status, &j.Attempts, &j.LastError, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

// ---- Profiles ----
func (s *Store) CreateProfile(ctx context.Context, tenantID int64, p *Profile) (*Profile, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO hotspot_profiles (tenant_id, name, rx_rate, tx_rate, session_uptime_limit, keepalive_timeout, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+profileCols,
		tenantID, p.Name, p.RxRate, p.TxRate, p.SessionUptimeLimit, p.KeepaliveTimeout, p.CreatedBy)
	created, err := scanProfile(row)
	return created, err
}

func (s *Store) ListProfiles(ctx context.Context, tenantID int64) ([]Profile, error) {
	rows, err := s.Query(ctx, "SELECT "+profileCols+" FROM hotspot_profiles WHERE tenant_id = $1 ORDER BY id", tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) ProfileByID(ctx context.Context, tenantID, id int64) (*Profile, error) {
	row := s.QueryRow(ctx, "SELECT "+profileCols+" FROM hotspot_profiles WHERE tenant_id = $1 AND id = $2", tenantID, id)
	p, err := scanProfile(row)
	return p, noRows(err)
}

func (s *Store) UpdateProfile(ctx context.Context, tenantID, id int64, p *Profile) error {
	_, err := s.Exec(ctx, `
		UPDATE hotspot_profiles SET name=$1, rx_rate=$2, tx_rate=$3, session_uptime_limit=$4, keepalive_timeout=$5, updated_at=now()
		WHERE tenant_id=$6 AND id=$7`, p.Name, p.RxRate, p.TxRate, p.SessionUptimeLimit, p.KeepaliveTimeout, tenantID, id)
	return err
}

// ---- Routers ----
func (s *Store) CreateRouter(ctx context.Context, tenantID int64, r *Router) (*Router, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO routers (tenant_id, name, ip_address, api_port, username, password_enc, status)
		VALUES ($1,$2,$3::inet,$4,$5,$6,'unverified') RETURNING `+routerCols,
		tenantID, r.Name, r.IPAddress, r.APIPort, r.Username, r.Username+"_encrypted")
	created, err := scanRouter(row)
	return created, err
}

func (s *Store) ListRouters(ctx context.Context, tenantID int64, status string) ([]Router, error) {
	rows, err := s.Query(ctx, "SELECT "+routerCols+
		" FROM routers WHERE tenant_id=$1 AND ($2='' OR status=$2) ORDER BY id", tenantID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Router
	for rows.Next() {
		r, err := scanRouter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *Store) RouterByID(ctx context.Context, tenantID, id int64) (*Router, error) {
	row := s.QueryRow(ctx, "SELECT "+routerCols+" FROM routers WHERE tenant_id=$1 AND id=$2", tenantID, id)
	r, err := scanRouter(row)
	return r, noRows(err)
}

func (s *Store) UpdateRouter(ctx context.Context, tenantID, id int64, r *Router) (*Router, error) {
	row := s.QueryRow(ctx, `
		UPDATE routers SET name=$1, ip_address=$2::inet, api_port=$3, username=$4, status='unverified', updated_at=now()
		WHERE tenant_id=$5 AND id=$6 RETURNING `+routerCols,
		r.Name, r.IPAddress, r.APIPort, r.Username, tenantID, id)
	updated, err := scanRouter(row)
	return updated, noRows(err)
}

func (s *Store) DeleteRouter(ctx context.Context, tenantID, id int64) (bool, error) {
	res, err := s.Exec(ctx, "DELETE FROM routers WHERE tenant_id=$1 AND id=$2", tenantID, id)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

func (s *Store) SetRouterStatus(ctx context.Context, id int64, status string) error {
	_, err := s.Exec(ctx, `UPDATE routers SET status=$1, last_seen_at=now() WHERE id=$2 AND status<>$1`, status, id)
	return err
}

func (s *Store) SetRouterOnline(ctx context.Context, id int64) error {
	_, err := s.Exec(ctx, `UPDATE routers SET status='online', last_seen_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) SetRouterOffline(ctx context.Context, id int64) error {
	_, err := s.Exec(ctx, "UPDATE routers SET status='offline' WHERE id=$1 AND status<>'offline'", id)
	return err
}

// SetRouterLastSync records the last successful poll time of a live router.
func (s *Store) SetRouterLastSync(ctx context.Context, id int64, ts time.Time) error {
	_, err := s.Exec(ctx, `UPDATE routers SET last_sync_at=$1 WHERE id=$2`, ts, id)
	return err
}

// RoutersAll returns every router regardless of tenant (worker loops).
func (s *Store) RoutersAll(ctx context.Context) ([]Router, error) {
	rows, err := s.Query(ctx, "SELECT "+routerCols+" FROM routers ORDER BY tenant_id, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Router
	for rows.Next() {
		r, err := scanRouter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// ---- Hotspots ----
func (s *Store) CreateHotspot(ctx context.Context, tenantID int64, h *Hotspot) (*Hotspot, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO hotspots (tenant_id, router_id, profile_id, name, ip_range, status)
		VALUES ($1,$2,$3,$4,$5,'configuring')
		RETURNING `+hotspotCols,
		tenantID, h.RouterID, h.ProfileID, h.Name, nilIfEmpty(h.IPRange))
	created, err := scanHotspot(row)
	return created, err
}

func (s *Store) ListHotspots(ctx context.Context, tenantID int64, routerID int64, status string) ([]Hotspot, error) {
	rows, err := s.Query(ctx, "SELECT "+hotspotCols+
		" FROM hotspots WHERE tenant_id=$1 AND ($2=0 OR router_id=$2) AND ($3='' OR status=$3) ORDER BY id",
		tenantID, routerID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Hotspot
	for rows.Next() {
		h, err := scanHotspot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

func (s *Store) HotspotByID(ctx context.Context, tenantID, id int64) (*Hotspot, error) {
	row := s.QueryRow(ctx, "SELECT "+hotspotCols+" FROM hotspots WHERE tenant_id=$1 AND id=$2", tenantID, id)
	h, err := scanHotspot(row)
	return h, noRows(err)
}

func (s *Store) UpdateHotspot(ctx context.Context, tenantID, id int64, h *Hotspot) (*Hotspot, error) {
	row := s.QueryRow(ctx, `
		UPDATE hotspots SET name=$1, router_id=$2, profile_id=$3, ip_range=$4, status='configuring', updated_at=now()
		WHERE tenant_id=$5 AND id=$6 RETURNING `+hotspotCols,
		h.Name, h.RouterID, h.ProfileID, nilIfEmpty(h.IPRange), tenantID, id)
	updated, err := scanHotspot(row)
	return updated, noRows(err)
}

func (s *Store) SetHotspotStatus(ctx context.Context, tenantID, id int64, status string) error {
	_, err := s.Exec(ctx, "UPDATE hotspots SET status=$1, updated_at=now() WHERE tenant_id=$2 AND id=$3", status, tenantID, id)
	return err
}

func (s *Store) SetHotspotConfigured(ctx context.Context, id int64, mikrotikID string, multiplier float64) error {
	_, err := s.Exec(ctx, `UPDATE hotspots SET status='active', mikrotik_id=COALESCE(NULLIF($2,''), mikrotik_id),
		applied_multiplier=$3, last_config_at=now(), updated_at=now() WHERE id=$1`, id, mikrotikID, multiplier)
	return err
}

func (s *Store) HotspotActiveSessionsCount(ctx context.Context, tenantID, id int64) (int64, error) {
	var n int64
	err := s.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE tenant_id=$1 AND hotspot_id=$2 AND state='active'", tenantID, id).Scan(&n)
	return n, err
}

// ---- Config jobs ----
func (s *Store) EnqueueJob(ctx context.Context, tenantID, hotspotID int64, action string, payload map[string]any) error {
	_, err := s.Exec(ctx, `
		INSERT INTO config_jobs (tenant_id, hotspot_id, action, payload)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT DO NOTHING`, tenantID, hotspotID, action, payload)
	return err
}

func (s *Store) PendingJobs(ctx context.Context, limit int) ([]ConfigJob, error) {
	rows, err := s.Query(ctx, "SELECT "+jobCols+" FROM config_jobs WHERE status='pending' ORDER BY id LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConfigJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

func (s *Store) DoneJob(ctx context.Context, id int64) error {
	_, err := s.Exec(ctx, "UPDATE config_jobs SET status='done', updated_at=now() WHERE id=$1", id)
	return err
}

func (s *Store) FailJob(ctx context.Context, id int64, message string) error {
	_, err := s.Exec(ctx, "UPDATE config_jobs SET status='failed', last_error=$1, attempts=attempts+1, updated_at=now() WHERE id=$2 AND status='pending'", message, id)
	return err
}

func nilIfEmpty(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}
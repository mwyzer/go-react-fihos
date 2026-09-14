package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type RateWindow struct {
	ID              int64     `json:"id"`
	TenantID        int64     `json:"tenant_id"`
	HotspotID       int64     `json:"hotspot_id"`
	HotspotName     string    `json:"hotspot_name,omitempty"`
	BoostMultiplier float64   `json:"boost_multiplier"`
	EffectiveFrom   time.Time `json:"effective_from"`
	EffectiveUntil  time.Time `json:"effective_until"`
	Repeat          map[string]any `json:"repeat"`
	Note            *string   `json:"note"`
	CreatedBy       *int64    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}

type AnomalyAlert struct {
	ID          int64          `json:"id"`
	TenantID    int64          `json:"tenant_id"`
	HotspotID   *int64         `json:"hotspot_id"`
	HotspotName string         `json:"hotspot_name,omitempty"`
	RouterID    *int64         `json:"router_id"`
	SessionID   *int64         `json:"session_id"`
	Type        string         `json:"type"`
	Severity    string         `json:"severity"`
	Status      string         `json:"status"`
	Value       float64        `json:"value"`
	Baseline    float64        `json:"baseline"`
	Fingerprint string         `json:"-"`
	Occurrences int            `json:"occurrences"`
	RateWindowID *int64        `json:"rate_window_id"`
	AckedAt     *time.Time     `json:"acked_at"`
	ResolvedAt  *time.Time     `json:"resolved_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type DailyRow struct {
	Day         time.Time `json:"day"`
	Sessions    int       `json:"sessions"`
	VouchersSold int      `json:"vouchers_sold"`
	BytesRX     int64     `json:"bytes_rx"`
	BytesTX     int64     `json:"bytes_tx"`
	Amount      float64   `json:"amount"`
}

const rwCols = `id, tenant_id, hotspot_id, boost_multiplier::float8, effective_from, effective_until, repeat, note, created_by, created_at`
const alertCols = `id, tenant_id, hotspot_id, router_id, session_id, type, severity, status, value::float8,
	baseline::float8, fingerprint, occurrences, rate_window_id, acked_at, resolved_at, created_at, updated_at`

// ---- Rate windows ----
func (s *Store) CreateRateWindow(ctx context.Context, rw *RateWindow) (*RateWindow, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO rate_windows (tenant_id, hotspot_id, boost_multiplier, effective_from, effective_until, repeat, note, created_by)
		VALUES ($1,$2,$3,$4,$5,COALESCE($6,'{}'::jsonb),$7,$8) RETURNING `+rwCols,
		rw.TenantID, rw.HotspotID, rw.BoostMultiplier, rw.EffectiveFrom, rw.EffectiveUntil, rw.Repeat, rw.Note, rw.CreatedBy)
	created := &RateWindow{}
	err := row.Scan(&created.ID, &created.TenantID, &created.HotspotID, &created.BoostMultiplier, &created.EffectiveFrom,
		&created.EffectiveUntil, &created.Repeat, &created.Note, &created.CreatedBy, &created.CreatedAt)
	return created, err
}

func (s *Store) ListRateWindows(ctx context.Context, tenantID, hotspotID int64, upcoming bool) ([]RateWindow, error) {
	where := "rw.tenant_id=$1 AND ($2=0 OR rw.hotspot_id=$2)"
	if upcoming {
		where += " AND rw.effective_until > now()"
	}
	rows, err := s.Query(ctx, `
		SELECT rw.id, rw.tenant_id, rw.hotspot_id, rw.boost_multiplier::float8, rw.effective_from, rw.effective_until, rw.repeat, rw.note, rw.created_by, rw.created_at, h.name
		FROM rate_windows rw JOIN hotspots h ON h.id=rw.hotspot_id AND h.tenant_id=rw.tenant_id
		WHERE `+where+` ORDER BY rw.effective_from DESC`, tenantID, hotspotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RateWindow = make([]RateWindow, 0)
	for rows.Next() {
		created := &RateWindow{}
		err := rows.Scan(&created.ID, &created.TenantID, &created.HotspotID, &created.BoostMultiplier, &created.EffectiveFrom,
			&created.EffectiveUntil, &created.Repeat, &created.Note, &created.CreatedBy, &created.CreatedAt, &created.HotspotName)
		if err != nil {
			return nil, err
		}
		out = append(out, *created)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRateWindow(ctx context.Context, tenantID, id int64) error {
	res, err := s.Exec(ctx, "DELETE FROM rate_windows WHERE tenant_id=$1 AND id=$2", tenantID, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type RateWindowJob struct {
	WindowID    int64
	TenantID    int64
	HotspotID   int64
	HotspotName string
	RouterID    int64
	Multiplier  float64
}

// WindowsToStart finds hotspots whose active rate window multiplier has not
// yet been applied to the simulated device.
func (s *Store) WindowsToStart(ctx context.Context) ([]RateWindowJob, error) {
	rows, err := s.Query(ctx, `
		SELECT rw.id, rw.tenant_id, h.id, h.name, h.router_id, rw.boost_multiplier::float8
		FROM rate_windows rw
		JOIN hotspots h ON h.tenant_id = rw.tenant_id AND h.id = rw.hotspot_id
		WHERE rw.effective_from <= now() AND rw.effective_until > now()
		  AND h.status = 'active'
		  AND abs(h.applied_multiplier - rw.boost_multiplier) > 0.001`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RateWindowJob
	for rows.Next() {
		j := RateWindowJob{}
		if err := rows.Scan(&j.WindowID, &j.TenantID, &j.HotspotID, &j.HotspotName, &j.RouterID, &j.Multiplier); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) ActiveRateWindow(ctx context.Context, tenantID, hotspotID int64) (*RateWindow, error) {
	row := s.QueryRow(ctx, `
		SELECT `+rwCols+` FROM rate_windows
		WHERE tenant_id=$1 AND hotspot_id=$2
		  AND effective_from <= now() AND effective_until > now()
		ORDER BY effective_from DESC LIMIT 1`, tenantID, hotspotID)
	created := &RateWindow{}
	err := row.Scan(&created.ID, &created.TenantID, &created.HotspotID, &created.BoostMultiplier, &created.EffectiveFrom,
		&created.EffectiveUntil, &created.Repeat, &created.Note, &created.CreatedBy, &created.CreatedAt)
	return created, noRows(err)
}

// ListExpiredAppliedWindows finds hotspots whose applied multiplier should be reverted.
func (s *Store) ListExpiredAppliedWindows(ctx context.Context, upTo time.Time) ([]RateWindowJob, error) {
	rows, err := s.Query(ctx, `
		SELECT h.id, h.tenant_id, h.name, h.router_id, h.applied_multiplier::float8 FROM hotspots h
		WHERE h.applied_multiplier <> 1
		  AND NOT EXISTS (SELECT 1 FROM rate_windows rw
		      WHERE rw.hotspot_id = h.id AND rw.effective_from <= $1 AND rw.effective_until > $1)`, upTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RateWindowJob
	for rows.Next() {
		j := RateWindowJob{}
		if err := rows.Scan(&j.HotspotID, &j.TenantID, &j.HotspotName, &j.RouterID, &j.Multiplier); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ---- Anomaly alerts ----
// InsertAlert records an alert, collapsing repeats of the same fingerprint
// within a 1-hour window into one open alert with incremented occurrences.
func (s *Store) InsertAlert(ctx context.Context, a *AnomalyAlert) (*AnomalyAlert, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var existing int64
err = tx.QueryRow(ctx, `
			SELECT id FROM anomaly_alerts
			WHERE tenant_id=$1 AND fingerprint=$2 AND status='open'
			  AND created_at > now() - interval '1 hour'`, a.TenantID, a.Fingerprint).Scan(&existing)
		if err == nil {
			row := tx.QueryRow(ctx, `UPDATE anomaly_alerts
				SET occurrences=occurrences+1, value=$1, updated_at=now()
				WHERE id=$2 RETURNING `+alertCols, a.Value, existing)
			out, scanErr := scanAlert(row)
			if scanErr != nil {
				return nil, scanErr
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return out, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

	row := tx.QueryRow(ctx, `
		INSERT INTO anomaly_alerts (tenant_id, hotspot_id, router_id, session_id, type, severity, status,
			value, baseline, fingerprint, rate_window_id)
		VALUES ($1,$2,$3,$4,$5,$6,'open',$7,$8,$9,$10)
		RETURNING `+alertCols,
		a.TenantID, a.HotspotID, a.RouterID, a.SessionID, a.Type, a.Severity, a.Value, a.Baseline, a.Fingerprint, a.RateWindowID)
	out, err := scanAlert(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func scanAlert(row interface{ Scan(...any) error }) (*AnomalyAlert, error) {
	a := &AnomalyAlert{}
	err := row.Scan(&a.ID, &a.TenantID, &a.HotspotID, &a.RouterID, &a.SessionID, &a.Type, &a.Severity, &a.Status,
		&a.Value, &a.Baseline, &a.Fingerprint, &a.Occurrences, &a.RateWindowID, &a.AckedAt, &a.ResolvedAt, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func isNoRowsErr(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}

func (s *Store) ListAlerts(ctx context.Context, tenantID int64, status, severity string, hotspotID int64, page, size int) ([]AnomalyAlert, int64, error) {
	where := "WHERE a.tenant_id=$1 AND ($2='' OR a.status=$2) AND ($3='' OR a.severity=$3) AND ($4=0 OR a.hotspot_id=$4)"
	var total int64
	if err := s.QueryRow(ctx, "SELECT count(*) FROM anomaly_alerts a "+where, tenantID, status, severity, hotspotID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Query(ctx, `
		SELECT a.id, a.tenant_id, a.hotspot_id, a.router_id, a.session_id, a.type, a.severity, a.status,
		       a.value::float8, a.baseline::float8, a.fingerprint, a.occurrences, a.rate_window_id, a.acked_at, a.resolved_at, a.created_at, a.updated_at,
		       COALESCE(h.name,'') AS hotspot_name
		FROM anomaly_alerts a LEFT JOIN hotspots h ON h.id=a.hotspot_id AND h.tenant_id=a.tenant_id
		`+where+` ORDER BY a.created_at DESC LIMIT $5 OFFSET $6`,
		tenantID, status, severity, hotspotID, size, Offset(page, size))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AnomalyAlert
	for rows.Next() {
		a := &AnomalyAlert{}
		err := rows.Scan(&a.ID, &a.TenantID, &a.HotspotID, &a.RouterID, &a.SessionID, &a.Type, &a.Severity, &a.Status,
			&a.Value, &a.Baseline, &a.Fingerprint, &a.Occurrences, &a.RateWindowID, &a.AckedAt, &a.ResolvedAt, &a.CreatedAt, &a.UpdatedAt,
			&a.HotspotName)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *a)
	}
	return out, total, rows.Err()
}

func (s *Store) UpdateAlertStatus(ctx context.Context, tenantID, id int64, status string) (*AnomalyAlert, error) {
	col := ""
	switch status {
	case "acknowledged":
		col = ", acked_at=now()"
	case "resolved":
		col = ", resolved_at=now()"
	case "dismissed":
		col = ", acked_at=now()"
	}
	row := s.QueryRow(ctx, "UPDATE anomaly_alerts SET status=$1, updated_at=now()"+col+""+
		" WHERE tenant_id=$2 AND id=$3 RETURNING "+alertCols, status, tenantID, id)
	a, err := scanAlert(row)
	return a, noRows(err)
}

func (s *Store) UpsertAnomalySummary(ctx context.Context, tenantID, hotspotID int64, t, severity string, bucketStart time.Time, openCount, total, resolved int, valueSum float64, first, last time.Time) error {
	_, err := s.Exec(ctx, `
		INSERT INTO anomaly_summaries (tenant_id, hotspot_id, type, severity, bucket_start, open_count, total_count, resolved_count, value_sum, first_seen, last_seen)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (tenant_id, hotspot_id, type, severity, bucket_start) DO UPDATE
		SET open_count=EXCLUDED.open_count, total_count=EXCLUDED.total_count, resolved_count=EXCLUDED.resolved_count,
		    value_sum=EXCLUDED.value_sum, first_seen=EXCLUDED.first_seen, last_seen=EXCLUDED.last_seen`,
		tenantID, hotspotID, t, severity, bucketStart, openCount, total, resolved, valueSum, first, last)
	return err
}

// ---- Analytics ----
type Dashboard struct {
	SessionsActive int     `json:"sessions_active"`
	SessionsToday  int     `json:"sessions_today"`
	VouchersSold   int     `json:"vouchers_sold"`
	RevenueToday   float64 `json:"revenue_today"`
	TrafficToday   int64   `json:"traffic_today"`
	TotalHotspots  int     `json:"total_hotspots"`
	ActiveHotspots int     `json:"active_hotspots"`
	OnlineRouters  int     `json:"online_routers"`
	OpenAlerts     int     `json:"open_alerts"`
}

func (s *Store) Dashboard(ctx context.Context, tenantID int64) (*Dashboard, error) {
	d := &Dashboard{}
	row := s.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM sessions WHERE tenant_id=$1 AND state='active'),
		  (SELECT count(*) FROM sessions WHERE tenant_id=$1 AND started_at >= date_trunc('day', now())),
		  (SELECT count(*) FROM vouchers WHERE tenant_id=$1 AND redeemed_at >= date_trunc('day', now())),
		  (SELECT COALESCE(sum(amount),0) FROM payments WHERE tenant_id=$1 AND status='succeeded' AND created_at >= date_trunc('day', now())),
		  (SELECT COALESCE(sum(u.bytes_rx + u.bytes_tx),0) FROM usage_logs u JOIN sessions se ON se.id=u.session_id WHERE se.tenant_id=$1 AND u.started_at >= date_trunc('day', now())),
		  (SELECT count(*) FROM hotspots WHERE tenant_id=$1),
		  (SELECT count(*) FROM hotspots WHERE tenant_id=$1 AND status='active'),
		  (SELECT count(*) FROM routers WHERE tenant_id=$1 AND status='online'),
		  (SELECT count(*) FROM anomaly_alerts WHERE tenant_id=$1 AND status='open')`, tenantID)
	err := row.Scan(&d.SessionsActive, &d.SessionsToday, &d.VouchersSold, &d.RevenueToday, &d.TrafficToday,
		&d.TotalHotspots, &d.ActiveHotspots, &d.OnlineRouters, &d.OpenAlerts)
	return d, err
}

func (s *Store) RollupDailies(ctx context.Context, tenantID int64, days int) error {
	_, err := s.Exec(ctx, `
		INSERT INTO daily_metrics (tenant_id, day, sessions, vouchers_sold, bytes_rx, bytes_tx, amount)
		SELECT t.id, d.day,
		  (SELECT count(*) FROM sessions se WHERE se.tenant_id=t.id AND se.started_at::date=d.day),
		  (SELECT count(*) FROM vouchers v WHERE v.tenant_id=t.id AND v.redeemed_at::date=d.day),
		  (SELECT COALESCE(sum(u.bytes_rx),0) FROM usage_logs u JOIN sessions se ON se.id=u.session_id WHERE se.tenant_id=t.id AND u.started_at::date=d.day),
		  (SELECT COALESCE(sum(u.bytes_tx),0) FROM usage_logs u JOIN sessions se ON se.id=u.session_id WHERE se.tenant_id=t.id AND u.started_at::date=d.day),
		  (SELECT COALESCE(sum(p.amount),0) FROM payments p WHERE p.tenant_id=t.id AND p.status='succeeded' AND p.created_at::date=d.day)
		FROM generate_series(current_date - ($1::int - 1), current_date, '1 day'::interval) AS d(day)
		CROSS JOIN tenants t WHERE t.id = $2
		ON CONFLICT (tenant_id, day) DO UPDATE
		SET sessions=EXCLUDED.sessions, vouchers_sold=EXCLUDED.vouchers_sold,
		    bytes_rx=EXCLUDED.bytes_rx, bytes_tx=EXCLUDED.bytes_tx, amount=EXCLUDED.amount`, days, tenantID)
	return err
}

func (s *Store) ReadDailyMetrics(ctx context.Context, tenantID int64, days int) ([]DailyRow, error) {
	rows, err := s.Query(ctx, `
		SELECT day, sessions, vouchers_sold, bytes_rx, bytes_tx, amount::float8
		FROM daily_metrics WHERE tenant_id=$1 AND day >= current_date - ($2::int - 1)
		ORDER BY day`, tenantID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyRow
	for rows.Next() {
		r := DailyRow{}
		if err := rows.Scan(&r.Day, &r.Sessions, &r.VouchersSold, &r.BytesRX, &r.BytesTX, &r.Amount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) HotspotTrafficSeries(ctx context.Context, tenantID, hotspotID int64, days int) ([]DailyRow, error) {
	rows, err := s.Query(ctx, `
		SELECT u.started_at::date, count(DISTINCT u.session_id)::int,
		       COALESCE(sum(u.bytes_rx),0), COALESCE(sum(u.bytes_tx),0)
		FROM usage_logs u JOIN sessions se ON se.id=u.session_id
		WHERE se.tenant_id=$1 AND ($2=0 OR se.hotspot_id=$2)
		  AND u.started_at >= current_date - ($3::int - 1)
		GROUP BY u.started_at::date ORDER BY u.started_at::date`, tenantID, hotspotID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyRow = make([]DailyRow, 0)
	for rows.Next() {
		r := DailyRow{}
		if err := rows.Scan(&r.Day, &r.Sessions, &r.BytesRX, &r.BytesTX); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type TopHotspot struct {
	HotspotID   int64   `json:"hotspot_id"`
	Name        string  `json:"name"`
	BytesRX     int64   `json:"bytes_rx"`
	BytesTX     int64   `json:"bytes_tx"`
	Sessions    int     `json:"sessions"`
}

func (s *Store) TopHotspots(ctx context.Context, tenantID int64, since time.Time, limit int) ([]TopHotspot, error) {
	rows, err := s.Query(ctx, `
		SELECT h.id, h.name, COALESCE(sum(u.bytes_rx),0), COALESCE(sum(u.bytes_tx),0), count(DISTINCT u.session_id)::int
		FROM usage_logs u JOIN sessions se ON se.id=u.session_id
		JOIN hotspots h ON h.id=se.hotspot_id AND h.tenant_id=se.tenant_id
		WHERE se.tenant_id=$1 AND u.started_at >= $2
		GROUP BY h.id, h.name ORDER BY COALESCE(sum(u.bytes_rx + u.bytes_tx),0) DESC LIMIT $3`,
		tenantID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopHotspot = make([]TopHotspot, 0)
	for rows.Next() {
		t := TopHotspot{}
		if err := rows.Scan(&t.HotspotID, &t.Name, &t.BytesRX, &t.BytesTX, &t.Sessions); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- Anomaly context ----

type SessionSnapshot struct {
	SessionID        int64
	HotspotID        int64
	VoucherID        *int64
	RouterID         int64
	StartTime        time.Time
	BytesRX          int64
	BytesTX          int64
	MacAddress       *string
	ProfileRXRate    int64
	ProfileTXRate    int64
	UptimeLimitMin   int
	RateWindowID     *int64
	ActiveRWMultiplier float64
}

type HotspotConcurrency struct {
	HotspotID    int64
	Active       int
	DistinctMAC  int
}

type HotspotHourly struct {
	HotspotID int64
	Bucket    time.Time
	Bytes     int64
	Sessions  int
}

// ActiveSessionsSnapshot joins sessions to their profile limits for the anomaly engine.
func (s *Store) ActiveSessionsSnapshot(ctx context.Context, tenantID int64) ([]SessionSnapshot, error) {
	rows, err := s.Query(ctx, `
		SELECT se.id, se.hotspot_id, se.voucher_id, se.router_id, se.started_at, se.bytes_rx, se.bytes_tx, se.mac_address,
		       p.rx_rate, p.tx_rate, p.session_uptime_limit, se.rate_window_id,
		       COALESCE((SELECT rw.boost_multiplier FROM rate_windows rw
		                 WHERE rw.hotspot_id = se.hotspot_id AND rw.effective_from <= now() AND rw.effective_until > now()
		                 ORDER BY rw.effective_from DESC LIMIT 1), 1)::float8
		FROM sessions se
		JOIN hotspot_profiles p ON p.tenant_id = se.tenant_id AND p.id = se.profile_id
		WHERE se.tenant_id = $1 AND se.state = 'active'`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionSnapshot
	for rows.Next() {
		snap := SessionSnapshot{}
		if err := rows.Scan(&snap.SessionID, &snap.HotspotID, &snap.VoucherID, &snap.RouterID, &snap.StartTime,
			&snap.BytesRX, &snap.BytesTX, &snap.MacAddress, &snap.ProfileRXRate, &snap.ProfileTXRate,
			&snap.UptimeLimitMin, &snap.RateWindowID, &snap.ActiveRWMultiplier); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) HotspotConcurrency(ctx context.Context, tenantID int64) ([]HotspotConcurrency, error) {
	rows, err := s.Query(ctx, `
		SELECT se.hotspot_id, count(*)::int, count(DISTINCT se.mac_address)::int
		FROM sessions se WHERE se.tenant_id = $1 AND se.state = 'active'
		GROUP BY se.hotspot_id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HotspotConcurrency
	for rows.Next() {
		hc := HotspotConcurrency{}
		if err := rows.Scan(&hc.HotspotID, &hc.Active, &hc.DistinctMAC); err != nil {
			return nil, err
		}
		out = append(out, hc)
	}
	return out, rows.Err()
}

// HotspotHourlyUsage returns per-hotspot hourly traffic for the last `days` days.
func (s *Store) HotspotHourlyUsage(ctx context.Context, tenantID int64, days int) ([]HotspotHourly, error) {
	rows, err := s.Query(ctx, `
		SELECT se.hotspot_id, date_trunc('hour', u.started_at) AS bucket,
		       COALESCE(sum(u.bytes_rx + u.bytes_tx), 0), count(*)::int
		FROM usage_logs u JOIN sessions se ON se.id = u.session_id
		WHERE se.tenant_id = $1 AND u.started_at >= now() - make_interval(days => $2::int)::interval
		GROUP BY se.hotspot_id, date_trunc('hour', u.started_at)
		ORDER BY se.hotspot_id, bucket`, tenantID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HotspotHourly
	for rows.Next() {
		hh := HotspotHourly{}
		if err := rows.Scan(&hh.HotspotID, &hh.Bucket, &hh.Bytes, &hh.Sessions); err != nil {
			return nil, err
		}
		out = append(out, hh)
	}
	return out, rows.Err()
}

type MacHopRow struct {
	Username string
	Macs     int
}

// DistinctMACCounts counts distinct MACs per username over the last `hours`.
func (s *Store) DistinctMACCounts(ctx context.Context, tenantID int64, hours int) ([]MacHopRow, error) {
	rows, err := s.Query(ctx, `
		SELECT username, count(DISTINCT mac_address)::int
		FROM sessions
		WHERE tenant_id = $1 AND mac_address IS NOT NULL
		  AND started_at >= now() - make_interval(hours => $2::int)
		GROUP BY username HAVING count(DISTINCT mac_address) > 1
		ORDER BY count(DISTINCT mac_address) DESC`, tenantID, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MacHopRow
	for rows.Next() {
		row := MacHopRow{}
		if err := rows.Scan(&row.Username, &row.Macs); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
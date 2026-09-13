package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrConflict = errors.New("conflict")

type Session struct {
	ID           int64      `json:"id"`
	TenantID     int64      `json:"tenant_id"`
	HotspotID    int64      `json:"hotspot_id"`
	RouterID     int64      `json:"router_id"`
	VoucherID    *int64     `json:"voucher_id"`
	ProfileID    *int64     `json:"profile_id"`
	State        string     `json:"state"`
	Username     string     `json:"username"`
	Password     *string    `json:"-"`
	MacAddress   *string    `json:"mac_address"`
	IPAddress    *string    `json:"ip_address"`
	StartedAt    time.Time  `json:"started_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	EndTime      *time.Time `json:"end_time"`
	BytesRX      int64      `json:"bytes_rx"`
	BytesTX      int64      `json:"bytes_tx"`
	RateWindowID *int64     `json:"rate_window_id"`
}

type UsageLog struct {
	ID           int64     `json:"id"`
	TenantID     int64     `json:"tenant_id"`
	SessionID    int64     `json:"session_id"`
	VoucherID    *int64    `json:"voucher_id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	DurationSec  int       `json:"duration_sec"`
	BytesRX      int64     `json:"bytes_rx"`
	BytesTX      int64     `json:"bytes_tx"`
	RateWindowID *int64    `json:"rate_window_id"`
}

const sessionCols = `id, tenant_id, hotspot_id, router_id, voucher_id, profile_id, state, username, password,
	mac_address, ip_address::text, started_at, last_seen_at, end_time, bytes_rx, bytes_tx, rate_window_id`

const usageCols = `id, tenant_id, session_id, voucher_id, started_at, ended_at, duration_sec, bytes_rx, bytes_tx, rate_window_id`

func scanSession(row interface{ Scan(...any) error }) (*Session, error) {
	s := &Session{}
	err := row.Scan(&s.ID, &s.TenantID, &s.HotspotID, &s.RouterID, &s.VoucherID, &s.ProfileID, &s.State, &s.Username,
		&s.Password, &s.MacAddress, &s.IPAddress, &s.StartedAt, &s.LastSeenAt, &s.EndTime, &s.BytesRX, &s.BytesTX, &s.RateWindowID)
	return s, err
}

// RedeemVoucher atomically marks a voucher as redeemed and opens a session.
// Returns ErrConflict if the voucher is not usable.
func (s *Store) RedeemVoucher(ctx context.Context, tenantID int64, v *Voucher, hotspotID int64, mac *string, sessionUser, password string) (*Session, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var routerID, profileID int64
	err = tx.QueryRow(ctx, "SELECT router_id, profile_id FROM hotspots WHERE tenant_id=$1 AND id=$2", tenantID, hotspotID).
		Scan(&routerID, &profileID)
	if err != nil {
		return nil, noRows(err)
	}

	res, err := tx.Exec(ctx, `
		UPDATE vouchers SET status='redeemed', redeemed_at=now()
		WHERE tenant_id=$1 AND id=$2 AND status='unused'`, tenantID, v.ID)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		return nil, ErrConflict
	}

	var rw *int64
	err = tx.QueryRow(ctx, `
		SELECT rw.id FROM rate_windows rw
		WHERE rw.tenant_id=$1 AND rw.hotspot_id=$2
		  AND rw.effective_from <= now() AND rw.effective_until > now()
		ORDER BY rw.effective_from DESC LIMIT 1`, tenantID, hotspotID).Scan(&rw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var sessionID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO sessions (tenant_id, hotspot_id, router_id, voucher_id, profile_id, state, username, password, mac_address, rate_window_id)
		VALUES ($1,$2,$3,$4,$5,'active',$6,$7,$8,$9) RETURNING id`,
		tenantID, hotspotID, routerID, v.ID, profileID, sessionUser, password, mac, rw).Scan(&sessionID)
	if err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE id=$1", sessionID)
	sess, err := scanSession(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) SessionByID(ctx context.Context, tenantID, id int64) (*Session, error) {
	row := s.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE tenant_id=$1 AND id=$2", tenantID, id)
	sess, err := scanSession(row)
	return sess, noRows(err)
}

func (s *Store) ActiveSessionByUsername(ctx context.Context, tenantID, hotspotID int64, username string) (*Session, error) {
	row := s.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE tenant_id=$1 AND hotspot_id=$2 AND username=$3 AND state='active'",
		tenantID, hotspotID, username)
	sess, err := scanSession(row)
	return sess, noRows(err)
}

func (s *Store) ActiveSessionByVoucher(ctx context.Context, tenantID, voucherID int64) (*Session, error) {
	row := s.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE tenant_id=$1 AND voucher_id=$2 AND state='active'",
		tenantID, voucherID)
	sess, err := scanSession(row)
	return sess, noRows(err)
}

func (s *Store) ListSessions(ctx context.Context, tenantID int64, state string, hotspotID int64, search string, page, size int) ([]Session, int64, error) {
	where := "WHERE tenant_id=$1 AND ($2='' OR state=$2) AND ($3=0 OR hotspot_id=$3)"
	args := []any{tenantID, state, hotspotID}
	if search != "" {
		where += " AND (username ILIKE '%'||$4||'%' OR mac_address ILIKE '%'||$4||'%' OR ip_address::text ILIKE '%'||$4||'%')"
		args = append(args, search)
	}
	var total int64
	if err := s.QueryRow(ctx, "SELECT count(*) FROM sessions "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	base := len(args)
	args = append(args, size, Offset(page, size))
	rows, err := s.Query(ctx, "SELECT "+sessionCols+" FROM sessions "+where+
		fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", base+1, base+2), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *sess)
	}
	return out, total, rows.Err()
}

// ListActiveSessions returns sessions to reconcile, optionally for one hotspot.
func (s *Store) ListActiveSessions(ctx context.Context, limit int, hotspotID int64) ([]Session, error) {
	query := `SELECT ` + sessionCols + ` FROM sessions
		WHERE state = 'active' AND ($1 = 0 OR hotspot_id = $1)
		ORDER BY started_at LIMIT $2`
	rows, err := s.Query(ctx, query, hotspotID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sess)
	}
	return out, rows.Err()
}

// UpsertRouterSession syncs a live router session discovered by the worker.
// username encodes the profile: "<profileID>-<opaque>".
func (s *Store) UpsertRouterSession(ctx context.Context, tenantID, hotspotID, routerID int64, profileID int64, username string, mac, ip *string) (*Session, error) {
	var sessionID int64
	err := s.QueryRow(ctx, `
		SELECT id FROM sessions WHERE tenant_id=$1 AND hotspot_id=$2 AND username=$3 AND state='active'`,
		tenantID, hotspotID, username).Scan(&sessionID)
	if err == nil {
		_, err = s.Exec(ctx, "UPDATE sessions SET last_seen_at=now() WHERE id=$1", sessionID)
		if err != nil {
			return nil, err
		}
		row := s.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE id=$1", sessionID)
		sess, sErr := scanSession(row)
		return sess, sErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	err = s.QueryRow(ctx, `
		INSERT INTO sessions (tenant_id, hotspot_id, router_id, profile_id, state, username, mac_address, ip_address)
		VALUES ($1,$2,$3,$4,'active',$5,$6,$7::inet) RETURNING id`,
		tenantID, hotspotID, routerID, profileID, username, mac, ip).Scan(&sessionID)
	if err != nil {
		return nil, err
	}
	row := s.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE id=$1", sessionID)
	sess, sErr := scanSession(row)
	return sess, sErr
}

// SnapshotSessionUsage records cumulative byte deltas for an active session
// into a usage_log row without closing it.
func (s *Store) SnapshotSessionUsage(ctx context.Context, sessionID int64, at time.Time, bytesRX, bytesTX int64) (*Session, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE id=$1 AND state='active' FOR UPDATE", sessionID)
	sess, err := scanSession(row)
	if err != nil {
		return nil, noRows(err)
	}
	deltaRX := bytesRX - sess.BytesRX
	deltaTX := bytesTX - sess.BytesTX
	if deltaRX < 0 {
		deltaRX = 0
	}
	if deltaTX < 0 {
		deltaTX = 0
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO usage_logs (tenant_id, session_id, voucher_id, started_at, ended_at, duration_sec, bytes_rx, bytes_tx, rate_window_id)
		VALUES ($1,$2,$3,$4,$4,0,$5,$6,$7)
		ON CONFLICT (session_id, started_at) DO UPDATE
		SET bytes_rx = usage_logs.bytes_rx + EXCLUDED.bytes_rx,
		    bytes_tx = usage_logs.bytes_tx + EXCLUDED.bytes_tx,
		    ended_at = now()`,
		sess.TenantID, sessionID, sess.VoucherID, at, deltaRX, deltaTX, sess.RateWindowID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE sessions SET bytes_rx=$1, bytes_tx=$2, last_seen_at=$3 WHERE id=$4`,
		bytesRX, bytesTX, at, sessionID)
	if err != nil {
		return nil, err
	}
	sess.BytesRX = bytesRX
	sess.BytesTX = bytesTX
	sess.LastSeenAt = &at
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return sess, nil
}

// CloseSession persists final deltas, writes the closing usage_log and moves the session to closed.
func (s *Store) CloseSession(ctx context.Context, sessionID int64, endedAt time.Time, bytesRX, bytesTX int64) (*Session, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, "SELECT "+sessionCols+" FROM sessions WHERE id=$1 AND state='active' FOR UPDATE", sessionID)
	sess, err := scanSession(row)
	if err != nil {
		return nil, noRows(err)
	}

	deltaRX := bytesRX - sess.BytesRX
	deltaTX := bytesTX - sess.BytesTX
	if deltaRX < 0 {
		deltaRX = 0
	}
	if deltaTX < 0 {
		deltaTX = 0
	}
	if bytesRX < sess.BytesRX {
		bytesRX = sess.BytesRX
	}
	if bytesTX < sess.BytesTX {
		bytesTX = sess.BytesTX
	}

	duration := int(endedAt.Sub(sess.StartedAt).Seconds())
	_, err = tx.Exec(ctx, `
		INSERT INTO usage_logs (tenant_id, session_id, voucher_id, started_at, ended_at, duration_sec, bytes_rx, bytes_tx, rate_window_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (session_id, started_at) DO UPDATE
		SET ended_at = EXCLUDED.ended_at, duration_sec = EXCLUDED.duration_sec,
		    bytes_rx = usage_logs.bytes_rx + EXCLUDED.bytes_rx,
		    bytes_tx = usage_logs.bytes_tx + EXCLUDED.bytes_tx`,
		sess.TenantID, sessionID, sess.VoucherID, endedAt.Add(-time.Minute), endedAt, duration, deltaRX, deltaTX, sess.RateWindowID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE sessions SET state='closed', end_time=$1, bytes_rx=$2, bytes_tx=$3 WHERE id=$4`,
		endedAt, bytesRX, bytesTX, sessionID)
	if err != nil {
		return nil, err
	}
	sess.State = "closed"
	sess.EndTime = &endedAt
	sess.BytesRX = bytesRX
	sess.BytesTX = bytesTX
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) ListUsage(ctx context.Context, tenantID, sessionID int64, from, to time.Time, page, size int) ([]UsageLog, int64, error) {
	where := "WHERE tenant_id=$1"
	args := []any{tenantID}
	if sessionID > 0 {
		where += " AND session_id=$2"
		args = append(args, sessionID)
	}
	if !from.IsZero() {
		where += " AND started_at >= $3"
		args = append(args, from)
	}
	if !to.IsZero() {
		where += " AND started_at <= $4"
		args = append(args, to)
	}
	var total int64
	if err := s.QueryRow(ctx, "SELECT count(*) FROM usage_logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, Offset(page, size))
	rows, err := s.Query(ctx, "SELECT "+usageCols+" FROM usage_logs "+where+
		fmt.Sprintf(" ORDER BY started_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []UsageLog
	for rows.Next() {
		u := UsageLog{}
		err := rows.Scan(&u.ID, &u.TenantID, &u.SessionID, &u.VoucherID, &u.StartedAt, &u.EndedAt, &u.DurationSec, &u.BytesRX, &u.BytesTX, &u.RateWindowID)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}
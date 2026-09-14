package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type BillingWindow struct {
	ID          int64      `json:"id"`
	TenantID    int64      `json:"tenant_id"`
	CustomerID  int64      `json:"customer_id"`
	Customer    string     `json:"customer_name"`
	PeriodStart time.Time  `json:"period_start"`
	PeriodEnd   time.Time  `json:"period_end"`
	Amount      float64    `json:"amount"`
	Status      string     `json:"status"`
	IssuedAt    time.Time  `json:"issued_at"`
	PaidAt      *time.Time `json:"paid_at"`
}

const billingCols = `b.id, b.tenant_id, b.customer_id, c.name, b.period_start, b.period_end,
		b.amount, b.status, b.issued_at, b.paid_at`

func scanBilling(row interface{ Scan(...any) error }) (*BillingWindow, error) {
	b := &BillingWindow{}
	err := row.Scan(&b.ID, &b.TenantID, &b.CustomerID, &b.Customer, &b.PeriodStart, &b.PeriodEnd,
		&b.Amount, &b.Status, &b.IssuedAt, &b.PaidAt)
	return b, err
}

const defaultMonthlyFee = 150000.0

// MonthlyFee returns the tenant's monthly billing fee from settings, defaulting to 150000.
func (s *Store) MonthlyFee(ctx context.Context, tenantID int64) (float64, error) {
	var fee float64
	err := s.QueryRow(ctx, `
		SELECT COALESCE((payment_config->>'monthly_fee')::numeric, 0)
		FROM settings WHERE tenant_id=$1`, tenantID).Scan(&fee)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultMonthlyFee, nil
		}
		return defaultMonthlyFee, nil
	}
	if fee <= 0 {
		return defaultMonthlyFee, nil
	}
	return fee, nil
}

// GenerateMonthlyWindows creates a billing window for the current month for every
// customer with status 'token' that does not yet have a window for the period.
func (s *Store) GenerateMonthlyWindows(ctx context.Context, tenantID int64, now time.Time) (int, error) {
	fee, err := s.MonthlyFee(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	res, err := s.Exec(ctx, `
		INSERT INTO billing_windows (tenant_id, customer_id, period_start, period_end, amount, status, issued_at)
		SELECT c.tenant_id, c.id,
		       date_trunc('month', $1::timestamptz)::date,
		       (date_trunc('month', $1::timestamptz)::date + interval '1 month' - interval '1 day')::date,
		       $2, 'issued', now()
		FROM customers c
		WHERE c.tenant_id = $3 AND c.status = 'token'
		  AND NOT EXISTS (
		      SELECT 1 FROM billing_windows bw
		      WHERE bw.customer_id = c.id AND bw.period_start = date_trunc('month', $1::timestamptz)::date
		  )`, now, fee, tenantID)
	if err != nil {
		return 0, err
	}
	return int(res.RowsAffected()), nil
}

func (s *Store) ListBillingWindows(ctx context.Context, tenantID, customerID int64, status string, page, size int) ([]BillingWindow, int64, error) {
	var (
		windows []BillingWindow
		total   int64
	)
	base := "FROM billing_windows b JOIN customers c ON c.tenant_id=b.tenant_id AND c.id=b.customer_id " +
		"WHERE b.tenant_id=$1 AND ($2=0 OR b.customer_id=$2) AND ($3='' OR b.status=$3)"
	err := s.QueryRow(ctx, "SELECT count(*) "+base, tenantID, customerID, status).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.Query(ctx, "SELECT "+billingCols+" "+base+
		" ORDER BY b.period_start DESC LIMIT $4 OFFSET $5",
		tenantID, customerID, status, size, Offset(page, size))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		w, err := scanBilling(rows)
		if err != nil {
			return nil, 0, err
		}
		windows = append(windows, *w)
	}
	return windows, total, rows.Err()
}

func (s *Store) ListPendingWindows(ctx context.Context, tenantID int64) ([]BillingWindow, error) {
	rows, err := s.Query(ctx, "SELECT "+billingCols+
		" FROM billing_windows b JOIN customers c ON c.tenant_id=b.tenant_id AND c.id=b.customer_id "+
		" WHERE b.tenant_id=$1 AND b.status IN ('issued','overdue') AND b.paid_at IS NULL ORDER BY b.period_start",
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BillingWindow
	for rows.Next() {
		w, err := scanBilling(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func (s *Store) MarkWindowPaid(ctx context.Context, tenantID, id int64) (*BillingWindow, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	w, err := markWindowPaidTx(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return w, tx.Commit(ctx)
}

func markWindowPaidTx(ctx context.Context, tx pgx.Tx, tenantID, id int64) (*BillingWindow, error) {
	row := tx.QueryRow(ctx, `
		UPDATE billing_windows bw SET status='paid', paid_at=now()
		WHERE bw.tenant_id=$1 AND bw.id=$2
		RETURNING bw.id, bw.tenant_id, bw.customer_id,
		          (SELECT c.name FROM customers c WHERE c.tenant_id=bw.tenant_id AND c.id=bw.customer_id),
		          bw.period_start, bw.period_end, bw.amount, bw.status, bw.issued_at, bw.paid_at`, tenantID, id)
	w := &BillingWindow{}
	err := row.Scan(&w.ID, &w.TenantID, &w.CustomerID, &w.Customer, &w.PeriodStart, &w.PeriodEnd,
		&w.Amount, &w.Status, &w.IssuedAt, &w.PaidAt)
	if err != nil {
		return nil, noRows(err)
	}
	return w, nil
}

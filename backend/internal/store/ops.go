package store

import (
	"context"
	"time"
)

type Payment struct {
	ID            int64     `json:"id"`
	TenantID      int64     `json:"tenant_id"`
	VoucherID     *int64    `json:"voucher_id"`
	ExternalRef   string    `json:"external_ref"`
	Amount        float64   `json:"amount"`
	Fee           float64   `json:"fee"`
	NetAmount     float64   `json:"net_amount"`
	Status        string    `json:"status"`
	PaymentMethod string    `json:"payment_method"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AuditLog struct {
	ID         int64          `json:"id"`
	TenantID   *int64         `json:"tenant_id"`
	UserID     *int64         `json:"user_id"`
	Action     string         `json:"action"`
	EntityType *string        `json:"entity_type"`
	EntityID   *int64         `json:"entity_id"`
	Meta       map[string]any `json:"meta"`
	CreatedAt  time.Time      `json:"created_at"`
}

const paymentCols = `id, tenant_id, voucher_id, external_ref, amount::float8, fee::float8, net_amount::float8, status, payment_method, created_at, updated_at`
const auditCols = `id, tenant_id, user_id, action, entity_type, entity_id, meta, created_at`

func (s *Store) CreatePayment(ctx context.Context, p *Payment) (*Payment, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO payments (tenant_id, voucher_id, external_ref, amount, fee, net_amount, status, payment_method)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+paymentCols,
		p.TenantID, p.VoucherID, p.ExternalRef, p.Amount, p.Fee, p.NetAmount, p.Status, p.PaymentMethod)
	out := &Payment{}
	err := row.Scan(&out.ID, &out.TenantID, &out.VoucherID, &out.ExternalRef, &out.Amount, &out.Fee, &out.NetAmount, &out.Status, &out.PaymentMethod, &out.CreatedAt, &out.UpdatedAt)
	return out, err
}

func (s *Store) PaymentByRef(ctx context.Context, tenantID int64, ref string) (*Payment, error) {
	row := s.QueryRow(ctx, "SELECT "+paymentCols+" FROM payments WHERE tenant_id=$1 AND external_ref=$2", tenantID, ref)
	out := &Payment{}
	err := row.Scan(&out.ID, &out.TenantID, &out.VoucherID, &out.ExternalRef, &out.Amount, &out.Fee, &out.NetAmount, &out.Status, &out.PaymentMethod, &out.CreatedAt, &out.UpdatedAt)
	return out, noRows(err)
}

// PaymentByRefGlobal looks a payment up by external ref regardless of tenant;
// used by webhook settlement where the caller is the gateway, not a tenant.
func (s *Store) PaymentByRefGlobal(ctx context.Context, ref string) (*Payment, error) {
	row := s.QueryRow(ctx, "SELECT "+paymentCols+" FROM payments WHERE external_ref=$1", ref)
	out := &Payment{}
	err := row.Scan(&out.ID, &out.TenantID, &out.VoucherID, &out.ExternalRef, &out.Amount, &out.Fee, &out.NetAmount, &out.Status, &out.PaymentMethod, &out.CreatedAt, &out.UpdatedAt)
	return out, noRows(err)
}

func (s *Store) UpdatePaymentStatus(ctx context.Context, id int64, status string) error {
	res, err := s.Exec(ctx, "UPDATE payments SET status=$1, updated_at=now() WHERE id=$2 AND status IN ('pending','failed')", status, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListPayments(ctx context.Context, tenantID int64, status string, page, size int) ([]Payment, int64, error) {
	where := "WHERE tenant_id=$1 AND ($2='' OR status=$2)"
	args := []any{tenantID, status}
	var total int64
	if err := s.QueryRow(ctx, "SELECT count(*) FROM payments "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, Offset(page, size))
	rows, err := s.Query(ctx, "SELECT "+paymentCols+" FROM payments "+where+
		" ORDER BY created_at DESC LIMIT $3 OFFSET $4", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		p := Payment{}
		err := rows.Scan(&p.ID, &p.TenantID, &p.VoucherID, &p.ExternalRef, &p.Amount, &p.Fee, &p.NetAmount, &p.Status, &p.PaymentMethod, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (s *Store) Audit(ctx context.Context, a *AuditLog) error {
	_, err := s.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, entity_type, entity_id, meta)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		a.TenantID, a.UserID, a.Action, a.EntityType, a.EntityID, a.Meta)
	return err
}

func (s *Store) ListAudit(ctx context.Context, tenantID *int64, action string, userID *int64, page, size int) ([]AuditLog, int64, error) {
	where := "WHERE ($1::bigint IS NULL OR tenant_id=$1) AND ($2='' OR action=$2) AND ($3::bigint IS NULL OR user_id=$3)"
	args := []any{tenantID, action, userID}
	var total int64
	if err := s.QueryRow(ctx, "SELECT count(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, Offset(page, size))
	rows, err := s.Query(ctx, "SELECT "+auditCols+" FROM audit_logs "+where+
		" ORDER BY created_at DESC LIMIT $4 OFFSET $5", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AuditLog
	for rows.Next() {
		a := AuditLog{}
		err := rows.Scan(&a.ID, &a.TenantID, &a.UserID, &a.Action, &a.EntityType, &a.EntityID, &a.Meta, &a.CreatedAt)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

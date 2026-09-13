package store

import (
	"context"
	"fmt"
	"time"
)

type Batch struct {
	ID        int64      `json:"id"`
	TenantID  int64      `json:"tenant_id"`
	Name      string     `json:"name"`
	Quantity  int        `json:"quantity"`
	Price     *float64   `json:"price"`
	Duration  int        `json:"duration"`
	ProfileID int64      `json:"profile_id"`
	ValidFrom *time.Time `json:"valid_from"`
	ValidTo   *time.Time `json:"valid_to"`
	CreatedBy *int64     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

type BatchSummary struct {
	Batch
	Unused   int `json:"unused"`
	Redeemed int `json:"redeemed"`
	Expired  int `json:"expired"`
	Revoked  int `json:"revoked"`
}

type Voucher struct {
	ID         int64      `json:"id"`
	TenantID   int64      `json:"tenant_id"`
	BatchID    int64      `json:"batch_id"`
	Code       string     `json:"code"`
	Status     string     `json:"status"`
	RedeemedAt *time.Time `json:"redeemed_at"`
	RedeemedBy *int64     `json:"redeemed_by"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

const batchCols = `id, tenant_id, name, quantity, price::float8, duration, profile_id, valid_from, valid_to, created_by, created_at`
const voucherCols = `id, tenant_id, batch_id, code, status, redeemed_at, redeemed_by, expires_at, created_at`

func scanBatch(row interface{ Scan(...any) error }) (*Batch, error) {
	b := &Batch{}
	err := row.Scan(&b.ID, &b.TenantID, &b.Name, &b.Quantity, &b.Price, &b.Duration, &b.ProfileID, &b.ValidFrom, &b.ValidTo, &b.CreatedBy, &b.CreatedAt)
	return b, err
}

func scanVoucher(row interface{ Scan(...any) error }) (*Voucher, error) {
	v := &Voucher{}
	err := row.Scan(&v.ID, &v.TenantID, &v.BatchID, &v.Code, &v.Status, &v.RedeemedAt, &v.RedeemedBy, &v.ExpiresAt, &v.CreatedAt)
	return v, err
}

// CreateBatch inserts a batch and its voucher rows atomically.
func (s *Store) CreateBatch(ctx context.Context, tenantID int64, b *Batch, codes []string) (*Batch, []Voucher, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	var id int64
	var created time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO voucher_batches (tenant_id, name, quantity, price, duration, profile_id, valid_from, valid_to, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at`,
		tenantID, b.Name, b.Quantity, b.Price, b.Duration, b.ProfileID, b.ValidFrom, b.ValidTo, b.CreatedBy).Scan(&id, &created)
	if err != nil {
		return nil, nil, err
	}
	b.ID = id
	b.TenantID = tenantID
	b.CreatedAt = created

	vouchers := make([]Voucher, 0, len(codes))
	for _, code := range codes {
		row := tx.QueryRow(ctx, `
			INSERT INTO vouchers (tenant_id, batch_id, code, expires_at)
			VALUES ($1,$2,$3,$4) RETURNING `+voucherCols,
			tenantID, id, code, b.ValidTo)
		v, err := scanVoucher(row)
		if err != nil {
			return nil, nil, err
		}
		vouchers = append(vouchers, *v)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return b, vouchers, nil
}

func (s *Store) BatchByID(ctx context.Context, tenantID, id int64) (*Batch, error) {
	row := s.QueryRow(ctx, "SELECT "+batchCols+" FROM voucher_batches WHERE tenant_id=$1 AND id=$2", tenantID, id)
	b, err := scanBatch(row)
	return b, noRows(err)
}

func (s *Store) ListBatches(ctx context.Context, tenantID int64, page, size int) ([]BatchSummary, int64, error) {
	var total int64
	if err := s.QueryRow(ctx, "SELECT count(*) FROM voucher_batches WHERE tenant_id=$1", tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Query(ctx, `
		SELECT b.id, b.tenant_id, b.name, b.quantity, b.price::float8, b.duration, b.profile_id, b.valid_from, b.valid_to, b.created_by, b.created_at,
		   count(*) FILTER (WHERE v.status='unused') AS unused,
		   count(*) FILTER (WHERE v.status='redeemed') AS redeemed,
		   count(*) FILTER (WHERE v.status='expired') AS expired,
		   count(*) FILTER (WHERE v.status='revoked') AS revoked
		FROM voucher_batches b
		JOIN vouchers v ON v.batch_id = b.id AND v.tenant_id = b.tenant_id
		WHERE b.tenant_id = $1
		GROUP BY b.id
		ORDER BY b.id DESC LIMIT $2 OFFSET $3`, tenantID, size, Offset(page, size))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []BatchSummary
	for rows.Next() {
		bs := BatchSummary{}
		if err := rows.Scan(&bs.Batch.ID, &bs.Batch.TenantID, &bs.Batch.Name, &bs.Batch.Quantity, &bs.Batch.Price,
			&bs.Batch.Duration, &bs.Batch.ProfileID, &bs.Batch.ValidFrom, &bs.Batch.ValidTo, &bs.Batch.CreatedBy,
			&bs.Batch.CreatedAt, &bs.Unused, &bs.Redeemed, &bs.Expired, &bs.Revoked); err != nil {
			return nil, 0, err
		}
		out = append(out, bs)
	}
	return out, total, rows.Err()
}

func (s *Store) VoucherByCode(ctx context.Context, tenantID int64, code string) (*Voucher, error) {
	row := s.QueryRow(ctx, `SELECT `+voucherCols+` FROM vouchers
		WHERE tenant_id=$1 AND upper(replace(code,'-',''))=upper($2)`, tenantID, code)
	v, err := scanVoucher(row)
	return v, noRows(err)
}

func (s *Store) VoucherByID(ctx context.Context, tenantID, id int64) (*Voucher, error) {
	row := s.QueryRow(ctx, "SELECT "+voucherCols+" FROM vouchers WHERE tenant_id=$1 AND id=$2", tenantID, id)
	v, err := scanVoucher(row)
	return v, noRows(err)
}

func (s *Store) ListVouchers(ctx context.Context, tenantID, batchID int64, status, q string, page, size int) ([]Voucher, int64, error) {
	var total int64
	where := "WHERE tenant_id=$1 AND ($2=0 OR batch_id=$2) AND ($3='' OR status=$3)"
	args := []any{tenantID, batchID, status}
	if q != "" {
		where = "WHERE tenant_id=$1 AND ($2=0 OR batch_id=$2) AND ($3='' OR status=$3) AND upper(code) LIKE '%'||upper($4)||'%'"
		args = append(args, q)
	}
	if err := s.QueryRow(ctx, "SELECT count(*) FROM vouchers "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	base := len(args)
	args = append(args, size, Offset(page, size))
	rows, err := s.Query(ctx, "SELECT "+voucherCols+" FROM vouchers "+where+
		fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", base+1, base+2), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Voucher
	for rows.Next() {
		v, err := scanVoucher(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *v)
	}
	return out, total, rows.Err()
}

func (s *Store) RevokeVoucher(ctx context.Context, tenantID, id int64) error {
	res, err := s.Exec(ctx, "UPDATE vouchers SET status='revoked' WHERE tenant_id=$1 AND id=$2 AND status='unused'", tenantID, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) BatchSales(ctx context.Context, tenantID, batchID int64) (*BatchSummary, error) {
	bs := &BatchSummary{}
	row := s.QueryRow(ctx, `
		SELECT b.id, b.tenant_id, b.name, b.quantity, b.price::float8, b.duration, b.profile_id, b.valid_from, b.valid_to, b.created_by, b.created_at,
		   count(*) FILTER (WHERE v.status='unused'),
		   count(*) FILTER (WHERE v.status='redeemed'),
		   count(*) FILTER (WHERE v.status='expired'),
		   count(*) FILTER (WHERE v.status='revoked')
		FROM voucher_batches b JOIN vouchers v ON v.batch_id=b.id AND v.tenant_id=b.tenant_id
		WHERE b.tenant_id=$1 AND b.id=$2 GROUP BY b.id`, tenantID, batchID)
	err := row.Scan(&bs.Batch.ID, &bs.Batch.TenantID, &bs.Batch.Name, &bs.Batch.Quantity, &bs.Batch.Price,
		&bs.Batch.Duration, &bs.Batch.ProfileID, &bs.Batch.ValidFrom, &bs.Batch.ValidTo, &bs.Batch.CreatedBy,
		&bs.Batch.CreatedAt, &bs.Unused, &bs.Redeemed, &bs.Expired, &bs.Revoked)
	return bs, noRows(err)
}
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Tenant struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Slug      string          `json:"slug"`
	Branding  map[string]any  `json:"branding"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type TenantSettings struct {
	PortalTitle   string         `json:"portal_title"`
	PortalMessage string         `json:"portal_message"`
	PaymentConfig map[string]any `json:"payment_config"`
}

const tenantCols = `id, name, slug, branding, status, created_at, updated_at`

func scanTenant(row interface{ Scan(...any) error }) (*Tenant, error) {
	t := &Tenant{}
	err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.Branding, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *Store) TenantByID(ctx context.Context, id int64) (*Tenant, error) {
	row := s.QueryRow(ctx, "SELECT "+tenantCols+" FROM tenants WHERE id = $1", id)
	t, err := scanTenant(row)
	return t, noRows(err)
}

func (s *Store) TenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	row := s.QueryRow(ctx, "SELECT "+tenantCols+" FROM tenants WHERE slug = $1", slug)
	t, err := scanTenant(row)
	return t, noRows(err)
}

func (s *Store) CreateTenant(ctx context.Context, name, slug string, branding map[string]any) (*Tenant, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO tenants (name, slug, branding) VALUES ($1, $2, $3)
		RETURNING `+tenantCols, name, slug, branding)
	t, err := scanTenant(row)
	return t, err
}

func (s *Store) UpdateTenant(ctx context.Context, id int64, name string, branding map[string]any) error {
	_, err := s.Exec(ctx, `
		UPDATE tenants SET name = COALESCE(NULLIF($1,''), name),
		       branding = COALESCE($2, branding), updated_at = now()
		WHERE id = $3`, name, branding, id)
	return err
}

func (s *Store) UpdateTenantStatus(ctx context.Context, id int64, status string) error {
	res, err := s.Exec(ctx, "UPDATE tenants SET status = $1, updated_at = now() WHERE id = $2", status, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListTenants(ctx context.Context, status string, page, size int) ([]Tenant, int64, error) {
	var (
		tenants []Tenant
		total   int64
	)
	base := "FROM tenants WHERE ($1 = '') OR status = $1"
	err := s.QueryRow(ctx, "SELECT count(*) "+base, status).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.Query(ctx, "SELECT "+tenantCols+" "+base+
		" ORDER BY id DESC LIMIT $2 OFFSET $3", status, size, Offset(page, size))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, *t)
	}
	return tenants, total, rows.Err()
}

func (s *Store) Settings(ctx context.Context, tenantID int64) (*TenantSettings, error) {
	row := s.QueryRow(ctx, `
		SELECT portal_title, portal_message, payment_config
		FROM settings WHERE tenant_id = $1`, tenantID)
	st := &TenantSettings{}
	err := row.Scan(&st.PortalTitle, &st.PortalMessage, &st.PaymentConfig)
	if errors.Is(err, pgx.ErrNoRows) {
		return &TenantSettings{PortalTitle: "Free Wi-Fi", PaymentConfig: map[string]any{}}, nil
	}
	return st, err
}

func (s *Store) UpsertSettings(ctx context.Context, tenantID int64, title, message string) error {
	_, err := s.Exec(ctx, `
		INSERT INTO settings (tenant_id, portal_title, portal_message)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id) DO UPDATE
		SET portal_title = COALESCE(NULLIF(EXCLUDED.portal_title,''), settings.portal_title),
		    portal_message = COALESCE(NULLIF(EXCLUDED.portal_message,''), settings.portal_message),
		    updated_at = now()`, tenantID, title, message)
	return err
}

// ActiveTenantIDs lists tenants eligible for worker loops.
func (s *Store) ActiveTenantIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.Query(ctx, "SELECT id FROM tenants WHERE status='active' ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
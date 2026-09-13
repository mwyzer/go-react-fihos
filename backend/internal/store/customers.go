package store

import (
	"context"
	"time"
)

type Customer struct {
	ID        int64      `json:"id"`
	TenantID  int64      `json:"tenant_id"`
	HotspotID *int64     `json:"hotspot_id"`
	Name      string     `json:"name"`
	Phone     string     `json:"phone"`
	Address   string     `json:"address"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

const customerCols = `id, tenant_id, hotspot_id, name, phone, address, status, created_at, updated_at`

func scanCustomer(row interface{ Scan(...any) error }) (*Customer, error) {
	c := &Customer{}
	err := row.Scan(&c.ID, &c.TenantID, &c.HotspotID, &c.Name, &c.Phone, &c.Address, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) CustomerByID(ctx context.Context, tenantID, id int64) (*Customer, error) {
	row := s.QueryRow(ctx, "SELECT "+customerCols+" FROM customers WHERE tenant_id=$1 AND id=$2", tenantID, id)
	c, err := scanCustomer(row)
	return c, noRows(err)
}

func (s *Store) CreateCustomer(ctx context.Context, tenantID int64, c *Customer) (*Customer, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO customers (tenant_id, hotspot_id, name, phone, address, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+customerCols,
		tenantID, c.HotspotID, c.Name, c.Phone, c.Address, c.Status)
	created, err := scanCustomer(row)
	return created, err
}

func (s *Store) PatchCustomerStatus(ctx context.Context, tenantID, id int64, status string) error {
	_, err := s.Exec(ctx,
		"UPDATE customers SET status=$1, updated_at=now() WHERE tenant_id=$2 AND id=$3",
		status, tenantID, id)
	return err
}

func (s *Store) ListCustomers(ctx context.Context, tenantID int64, status string, page, size int) ([]Customer, int64, error) {
	var (
		customers []Customer
		total     int64
	)
	base := "FROM customers WHERE tenant_id=$1 AND ($2='' OR status=$2)"
	err := s.QueryRow(ctx, "SELECT count(*) "+base, tenantID, status).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.Query(ctx, "SELECT "+customerCols+" "+base+
		" ORDER BY id LIMIT $3 OFFSET $4",
		tenantID, status, size, Offset(page, size))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, 0, err
		}
		customers = append(customers, *c)
	}
	return customers, total, rows.Err()
}

// CountCustomersByStatus returns count per status for the given tenant.
func (s *Store) CountCustomersByStatus(ctx context.Context, tenantID int64) (map[string]int64, error) {
	rows, err := s.Query(ctx,
		"SELECT status, count(*) FROM customers WHERE tenant_id=$1 GROUP BY status", tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

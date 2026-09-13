package store

import (
	"context"
	"time"
)

type User struct {
	ID           int64      `json:"id"`
	TenantID     *int64     `json:"tenant_id"`
	Email        string     `json:"email"`
	FullName     string     `json:"full_name"`
	Role         string     `json:"role"`
	IsActive     bool       `json:"is_active"`
	PasswordHash string     `json:"-"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

const userCols = `id, tenant_id, email, password_hash, full_name, role, is_active, last_login_at, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &u.IsActive, &u.LastLoginAt, &u.CreatedAt)
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE email = $1", email)
	u, err := scanUser(row)
	return u, noRows(err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	row := s.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE id = $1", id)
	u, err := scanUser(row)
	return u, noRows(err)
}

func (s *Store) CreateUser(ctx context.Context, u *User, passwordHash string) (*User, error) {
	row := s.QueryRow(ctx, `
		INSERT INTO users (tenant_id, email, password_hash, full_name, role, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+userCols,
		u.TenantID, u.Email, passwordHash, u.FullName, u.Role, u.IsActive)
	created, err := scanUser(row)
	return created, err
}

func (s *Store) UpdateUser(ctx context.Context, id int64, tenantID *int64, fullName, role string, isActive *bool, email string) error {
	_, err := s.Exec(ctx, `
		UPDATE users SET full_name = $1, role = $2, is_active = COALESCE($3, is_active),
		       email = $4, updated_at = now()
		WHERE id = $5 AND (($6::bigint IS NULL) OR tenant_id = $6)`,
		fullName, role, isActive, email, id, tenantID)
	return err
}

func (s *Store) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := s.Exec(ctx, "UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2", passwordHash, id)
	return err
}

func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	_, err := s.Exec(ctx, "UPDATE users SET last_login_at = now() WHERE id = $1", id)
	return err
}

func (s *Store) ListUsers(ctx context.Context, tenantID *int64, page, size int) ([]User, int64, error) {
	var (
		users []User
		total int64
	)
	base := "FROM users WHERE ($1::bigint IS NULL) OR tenant_id = $1"
	err := s.QueryRow(ctx, "SELECT count(*) "+base, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.Query(ctx, "SELECT "+userCols+" "+base+
		" ORDER BY id DESC LIMIT $2 OFFSET $3", tenantID, size, Offset(page, size))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, *u)
	}
	return users, total, rows.Err()
}
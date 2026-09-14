package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrInsufficient = errors.New("insufficient balance")

type WalletTransaction struct {
	ID           int64     `json:"id"`
	TenantID     int64     `json:"tenant_id"`
	CustomerID   int64     `json:"customer_id"`
	Type         string    `json:"type"`
	Amount       float64   `json:"amount"`
	BalanceAfter float64   `json:"balance_after"`
	RefType      string    `json:"ref_type"`
	RefID        int64     `json:"ref_id"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

const walletTxCols = `id, tenant_id, customer_id, type, amount::float8,
	balance_after::float8, COALESCE(ref_type, ''), COALESCE(ref_id, 0), COALESCE(note, ''), created_at`

func scanWalletTx(row interface{ Scan(...any) error }) (*WalletTransaction, error) {
	t := &WalletTransaction{}
	err := row.Scan(&t.ID, &t.TenantID, &t.CustomerID, &t.Type, &t.Amount,
		&t.BalanceAfter, &t.RefType, &t.RefID, &t.Note, &t.CreatedAt)
	return t, err
}

func (s *Store) CustomerBalance(ctx context.Context, tenantID, customerID int64) (float64, error) {
	var balance float64
	err := s.QueryRow(ctx, "SELECT balance FROM customers WHERE tenant_id=$1 AND id=$2", tenantID, customerID).Scan(&balance)
	return balance, noRows(err)
}

func (s *Store) ListWalletTransactions(ctx context.Context, tenantID, customerID int64, page, size int) ([]WalletTransaction, int64, error) {
	where := "WHERE tenant_id=$1 AND ($2=0 OR customer_id=$2)"
	args := []any{tenantID, customerID}
	var total int64
	if err := s.QueryRow(ctx, "SELECT count(*) FROM wallet_transactions "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, Offset(page, size))
	rows, err := s.Query(ctx, "SELECT "+walletTxCols+" FROM wallet_transactions "+where+
		" ORDER BY id DESC LIMIT $3 OFFSET $4", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []WalletTransaction
	for rows.Next() {
		t, err := scanWalletTx(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	return out, total, rows.Err()
}

// CreditWallet adds a positive amount to a customer balance and records a
// ledger row (type 'topup') atomically.
func (s *Store) CreditWallet(ctx context.Context, tenantID, customerID int64, amount float64, refType string, refID int64, note string) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}
	return s.adjustBalance(ctx, tenantID, customerID, "topup", amount, refType, refID, note)
}

// AdjustWallet applies a signed manual correction to a customer balance.
// Errors with ErrInsufficient when a debit would go negative.
func (s *Store) AdjustWallet(ctx context.Context, tenantID, customerID int64, delta float64, note string) error {
	if delta == 0 {
		return errors.New("amount must not be zero")
	}
	return s.adjustBalance(ctx, tenantID, customerID, "adjustment", delta, "", 0, note)
}

// DebitWalletForBill deducts amount from a customer balance and marks the
// billing window paid. Errors with ErrInsufficient when balance is too low.
func (s *Store) DebitWalletForBill(ctx context.Context, tenantID, customerID, windowID int64, amount float64) (*BillingWindow, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	newBalance, err := lockAndApply(ctx, tx, tenantID, customerID, -amount)
	if err != nil {
		return nil, err
	}
	if err := insertWalletTx(ctx, tx, tenantID, customerID, "bill_payment", -amount, newBalance, "billing_window", windowID, ""); err != nil {
		return nil, err
	}
	w, err := markWindowPaidTx(ctx, tx, tenantID, windowID)
	if err != nil {
		return nil, err
	}
	return w, tx.Commit(ctx)
}

// DebitWalletForBillByWindow looks up a billing window and pays it from the
// owning customer's wallet atomically.
func (s *Store) DebitWalletForBillByWindow(ctx context.Context, tenantID, windowID int64) (int64, *BillingWindow, error) {
	tx, err := s.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)

	var customerID int64
	var amount float64
	if err := tx.QueryRow(ctx, `
		SELECT customer_id, amount FROM billing_windows
		WHERE tenant_id=$1 AND id=$2 AND status <> 'paid' FOR UPDATE`, tenantID, windowID).
		Scan(&customerID, &amount); err != nil {
		return 0, nil, noRows(err)
	}

	newBalance, err := lockAndApply(ctx, tx, tenantID, customerID, -amount)
	if err != nil {
		return customerID, nil, err
	}
	if err := insertWalletTx(ctx, tx, tenantID, customerID, "bill_payment", -amount, newBalance, "billing_window", windowID, ""); err != nil {
		return 0, nil, err
	}
	w, err := markWindowPaidTx(ctx, tx, tenantID, windowID)
	if err != nil {
		return 0, nil, err
	}
	return customerID, w, tx.Commit(ctx)
}

// adjustBalance applies a signed delta inside a transaction, enforcing the
// non-negative balance invariant, and records the ledger row.
func (s *Store) adjustBalance(ctx context.Context, tenantID, customerID int64, typ string, delta float64, refType string, refID int64, note string) error {
	tx, err := s.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	newBalance, err := lockAndApply(ctx, tx, tenantID, customerID, delta)
	if err != nil {
		return err
	}
	if err := insertWalletTx(ctx, tx, tenantID, customerID, typ, delta, newBalance, refType, refID, note); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// lockAndApply locks the customer row and applies a signed balance delta,
// returning the resulting balance.
func lockAndApply(ctx context.Context, tx pgx.Tx, tenantID, customerID int64, delta float64) (float64, error) {
	var balance float64
	if err := tx.QueryRow(ctx,
		"SELECT balance FROM customers WHERE tenant_id=$1 AND id=$2 FOR UPDATE", tenantID, customerID).
		Scan(&balance); err != nil {
		return 0, noRows(err)
	}
	if balance+delta < 0 {
		return 0, ErrInsufficient
	}
	if _, err := tx.Exec(ctx,
		"UPDATE customers SET balance = balance + $1, updated_at = now() WHERE tenant_id=$2 AND id=$3",
		delta, tenantID, customerID); err != nil {
		return 0, err
	}
	return balance + delta, nil
}

func insertWalletTx(ctx context.Context, tx pgx.Tx, tenantID, customerID int64, typ string, amount, balanceAfter float64, refType string, refID int64, note string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO wallet_transactions (tenant_id, customer_id, type, amount, balance_after, ref_type, ref_id, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		tenantID, customerID, typ, amount, balanceAfter, nullStr(refType), nullI64(refID), note)
	return err
}

func nullStr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func nullI64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

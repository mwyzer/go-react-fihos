package service

import (
	"context"
	"time"

	"fihos/backend/internal/store"
)

// Billing integrates a mock payment gateway. Paid vouchers mint a payment row
// that is immediately marked succeeded so revenue analytics have data; the
// seam is where a real gateway (Midtrans/Duitku/QRIS) would be plugged in.
type Billing struct {
	st *store.Store
}

func NewBilling(st *store.Store) *Billing {
	return &Billing{st: st}
}

// Charge creates and instantly succeeds a mock payment for a voucher.
func (b *Billing) Charge(ctx context.Context, tenantID int64, voucherID int64, amount float64) (*store.Payment, error) {
	net := amount * 0.97
	ref := ExternalRef("pay", voucherID)
	p, err := b.st.CreatePayment(ctx, &store.Payment{
		TenantID:      tenantID,
		VoucherID:     &voucherID,
		ExternalRef:   ref,
		Amount:        amount,
		Fee:           amount - net,
		NetAmount:     net,
		Status:        "pending",
		PaymentMethod: "mock",
	})
	if err != nil {
		return nil, err
	}
	if err := b.st.UpdatePaymentStatus(ctx, p.ID, "succeeded"); err != nil {
		return nil, err
	}
	p.Status = "succeeded"
	p.UpdatedAt = time.Now()
	return p, nil
}
package service

import (
	"context"
	"time"

	"fihos/backend/internal/store"
)

// ChargeResult reports the outcome of a gateway charge.
type ChargeResult struct {
	Payment    *store.Payment
	Async      bool
	PaymentURL string
}

// Billing integrates a payment gateway. Voucher charges mint a payment row and
// are either settled instantly (mock provider, the legacy demo behaviour) or
// left pending until a webhook reports settlement (async provider, real
// gateway flow).
type Billing struct {
	st       *store.Store
	provider Provider
	feeRate  float64
}

func NewBilling(st *store.Store, provider Provider) *Billing {
	return &Billing{st: st, provider: provider, feeRate: 0.03}
}

// Charge creates a payment for a voucher through the configured provider.
func (b *Billing) Charge(ctx context.Context, tenantID int64, voucherID int64, amount float64) (*ChargeResult, error) {
	ref := ExternalRef("pay", voucherID)
	res, err := b.provider.Create(ctx, ProviderRequest{
		TenantID:    tenantID,
		VoucherID:   voucherID,
		ExternalRef: ref,
		Amount:      amount,
		Description: "voucher",
	})
	if err != nil {
		return nil, err
	}

	net := amount * (1 - b.feeRate)
	p, err := b.st.CreatePayment(ctx, &store.Payment{
		TenantID:      tenantID,
		VoucherID:     &voucherID,
		ExternalRef:   ref,
		Amount:        amount,
		Fee:           amount - net,
		NetAmount:     net,
		Status:        "pending",
		PaymentMethod: res.PaymentMethod,
	})
	if err != nil {
		return nil, err
	}

	if !res.Async {
		if err := b.st.UpdatePaymentStatus(ctx, p.ID, "succeeded"); err != nil {
			return nil, err
		}
		p.Status = "succeeded"
		p.UpdatedAt = time.Now()
	}
	return &ChargeResult{Payment: p, Async: res.Async, PaymentURL: res.PaymentURL}, nil
}

// Settle marks a pending payment as succeeded after a webhook reports
// settlement. It is idempotent: already-settled payments are left untouched.
func (b *Billing) Settle(ctx context.Context, externalRef string) (*store.Payment, error) {
	p, err := b.st.PaymentByRefGlobal(ctx, externalRef)
	if err != nil {
		return nil, err
	}
	if p.Status == "succeeded" {
		return p, nil
	}
	if err := b.st.UpdatePaymentStatus(ctx, p.ID, "succeeded"); err != nil {
		return nil, err
	}
	p.Status = "succeeded"
	p.UpdatedAt = time.Now()
	return p, nil
}

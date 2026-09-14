package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Provider abstracts a payment gateway. The mock provider settles instantly
// (the behaviour the platform shipped with); the sandbox provider creates a
// pending payment that is settled through the webhook endpoint, mirroring how
// a real aggregation gateway (QRIS/VA/e-wallet) reports settlement.
type Provider interface {
	Create(ctx context.Context, req ProviderRequest) (*ProviderResult, error)
}

type ProviderRequest struct {
	TenantID    int64
	VoucherID   int64
	ExternalRef string
	Amount      float64
	Description string
}

type ProviderResult struct {
	// Async is true when settlement arrives out-of-band (webhook polling).
	Async bool
	// PaymentMethod is the gateway identifier recorded on the payment row.
	PaymentMethod string
	// PaymentURL points to the gateway checkout instructions for the customer.
	PaymentURL string
}

// ProviderConfig carries the settings used by non-mock providers.
type ProviderConfig struct {
	// SandboxBaseURL prefixes the generated payment URL for the sandbox
	// provider, e.g. "https://pay.sandbox.example".
	SandboxBaseURL string
}

func NewProvider(name string, cfg ProviderConfig) Provider {
	switch strings.ToLower(name) {
	case "sandbox":
		return &sandboxProvider{base: cfg.SandboxBaseURL}
	default:
		return &mockProvider{}
	}
}

// mockProvider settles immediately, keeping the old demo behaviour.
type mockProvider struct{}

func (p *mockProvider) Create(_ context.Context, _ ProviderRequest) (*ProviderResult, error) {
	return &ProviderResult{Async: false, PaymentMethod: "mock", PaymentURL: ""}, nil
}

// sandboxProvider leaves the payment pending and returns a checkout URL whose
// settlement is reported through POST /api/v1/payments/{ref}/complete.
type sandboxProvider struct {
	base string
}

func (p *sandboxProvider) Create(_ context.Context, req ProviderRequest) (*ProviderResult, error) {
	if p.base == "" {
		return nil, fmt.Errorf("sandbox provider: PAYMENT_SANDBOX_URL is not configured")
	}
	return &ProviderResult{
		Async:         true,
		PaymentMethod: "sandbox",
		PaymentURL:    strings.TrimRight(p.base, "/") + "/pay/" + req.ExternalRef,
	}, nil
}

// PaymentSignature computes an HMAC-SHA256 over the settlement payload
// (external_ref|amount). Real gateway webhooks carry their own scheme; this
// gives the built-in endpoints the same verify-before-settle discipline.
func PaymentSignature(secret, externalRef string, amount float64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s|%.2f", externalRef, amount)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyPaymentSignature(secret, externalRef string, amount float64, sig string) bool {
	return hmac.Equal([]byte(PaymentSignature(secret, externalRef, amount)), []byte(sig))
}

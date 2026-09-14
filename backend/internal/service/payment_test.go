package service

import (
	"context"
	"testing"
)

func TestMockProviderSettlesInstantly(t *testing.T) {
	p := NewProvider("mock", ProviderConfig{})
	res, err := p.Create(context.Background(), ProviderRequest{ExternalRef: "pay-1", Amount: 20000})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Async {
		t.Fatal("mock provider must settle instantly")
	}
	if res.PaymentMethod != "mock" {
		t.Fatalf("expected mock method, got %s", res.PaymentMethod)
	}
}

func TestSandboxProviderLeavesPendingWithURL(t *testing.T) {
	p := NewProvider("sandbox", ProviderConfig{SandboxBaseURL: "https://pay.sandbox.example/checkout"})
	res, err := p.Create(context.Background(), ProviderRequest{ExternalRef: "pay-9", Amount: 15000})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !res.Async {
		t.Fatal("sandbox provider must be async")
	}
	if want := "https://pay.sandbox.example/checkout/pay/pay-9"; res.PaymentURL != want {
		t.Fatalf("payment url = %q, want %q", res.PaymentURL, want)
	}
}

func TestSandboxProviderRequiresBaseURL(t *testing.T) {
	p := NewProvider("SANDBOX", ProviderConfig{})
	if _, err := p.Create(context.Background(), ProviderRequest{ExternalRef: "x"}); err == nil {
		t.Fatal("expected error when PAYMENT_SANDBOX_URL unset")
	}
}

func TestUnknownProviderFallsBackToMock(t *testing.T) {
	p := NewProvider("nope", ProviderConfig{})
	res, err := p.Create(context.Background(), ProviderRequest{ExternalRef: "y"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Async {
		t.Fatal("fallback provider must settle instantly")
	}
}

func TestPaymentSignatureRoundTrip(t *testing.T) {
	secret := "s3cret-webhook"
	ref := "pay-42"
	amount := 12345.67
	sig := PaymentSignature(secret, ref, amount)
	if !VerifyPaymentSignature(secret, ref, amount, sig) {
		t.Fatal("expected signature to verify")
	}
	if VerifyPaymentSignature(secret, ref, amount+0.01, sig) {
		t.Fatal("signature must not verify for a different amount")
	}
	if VerifyPaymentSignature(secret, ref+"x", amount, sig) {
		t.Fatal("signature must not verify for a different ref")
	}
	if VerifyPaymentSignature("other-secret", ref, amount, sig) {
		t.Fatal("signature must not verify for a different secret")
	}
}
package gateway

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSandboxCreateCheckout_Success(t *testing.T) {
	p := NewSandboxProvider("sandbox", "https://pay.local/checkout/", 2*time.Hour)
	fixed := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return fixed }

	res, err := p.CreateCheckout(context.Background(), CheckoutInput{
		OrderID:  "ORD-abc",
		Amount:   2000000,
		Currency: "IDR",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Provider != "sandbox" {
		t.Fatalf("provider = %q", res.Provider)
	}
	if res.ExternalOrderID != "ORD-abc" {
		t.Fatalf("external order id = %q", res.ExternalOrderID)
	}
	if res.CheckoutURL != "https://pay.local/checkout/ORD-abc" {
		t.Fatalf("checkout url = %q (trailing slash should be normalized)", res.CheckoutURL)
	}
	if !res.ExpiresAt.Equal(fixed.Add(2 * time.Hour)) {
		t.Fatalf("expires_at = %v, want now+ttl", res.ExpiresAt)
	}
	if !strings.Contains(string(res.RawResponse), "ORD-abc") {
		t.Fatalf("raw response should retain the order id: %s", res.RawResponse)
	}
}

func TestSandboxCreateCheckout_Validation(t *testing.T) {
	p := NewSandboxProvider("sandbox", "", 0)

	if _, err := p.CreateCheckout(context.Background(), CheckoutInput{Amount: 1000}); err == nil {
		t.Fatal("missing order id should error")
	}
	if _, err := p.CreateCheckout(context.Background(), CheckoutInput{OrderID: "ORD-1", Amount: 0}); err == nil {
		t.Fatal("non-positive amount should error")
	}
}

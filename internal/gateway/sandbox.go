package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SandboxProvider is a self-contained gateway provider used for local
// development and the MVP. It mints a deterministic checkout URL from the order
// ID instead of calling an external API, so the tenant Pay Now flow is fully
// exercisable end to end without provider credentials. Real providers implement
// the same Provider interface and replace this one via configuration.
type SandboxProvider struct {
	name            string
	checkoutBaseURL string
	ttl             time.Duration
	now             func() time.Time
}

// NewSandboxProvider builds a SandboxProvider. name is the provider identifier
// stored on transactions; checkoutBaseURL is the prefix for generated checkout
// links; ttl is how long a generated checkout stays valid.
func NewSandboxProvider(name, checkoutBaseURL string, ttl time.Duration) *SandboxProvider {
	if name == "" {
		name = "sandbox"
	}
	if checkoutBaseURL == "" {
		checkoutBaseURL = "https://sandbox.pay.local/checkout"
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &SandboxProvider{
		name:            name,
		checkoutBaseURL: strings.TrimRight(checkoutBaseURL, "/"),
		ttl:             ttl,
		now:             time.Now,
	}
}

// Name returns the provider identifier.
func (p *SandboxProvider) Name() string { return p.name }

// CreateCheckout synthesizes a checkout session for the given input.
func (p *SandboxProvider) CreateCheckout(_ context.Context, in CheckoutInput) (*CheckoutResult, error) {
	if in.OrderID == "" {
		return nil, fmt.Errorf("sandbox: order id is required")
	}
	if in.Amount <= 0 {
		return nil, fmt.Errorf("sandbox: amount must be positive")
	}

	externalTxID := "sbx_" + in.OrderID
	checkoutURL := fmt.Sprintf("%s/%s", p.checkoutBaseURL, in.OrderID)
	expiresAt := p.now().UTC().Add(p.ttl)

	raw, err := json.Marshal(map[string]any{
		"provider":                p.name,
		"order_id":                in.OrderID,
		"external_transaction_id": externalTxID,
		"amount":                  in.Amount,
		"currency":                in.Currency,
		"checkout_url":            checkoutURL,
		"expires_at":              expiresAt.Format(time.RFC3339),
		"return_url":              in.ReturnURL,
	})
	if err != nil {
		return nil, fmt.Errorf("sandbox: marshal raw response: %w", err)
	}

	return &CheckoutResult{
		Provider:              p.name,
		ExternalOrderID:       in.OrderID,
		ExternalTransactionID: externalTxID,
		CheckoutURL:           checkoutURL,
		ExpiresAt:             expiresAt,
		RawResponse:           raw,
	}, nil
}

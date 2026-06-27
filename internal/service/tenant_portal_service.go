package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iqbal-hamdani/go-backend/internal/gateway"
	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// ErrUnsupportedProvider is returned when a tenant requests a gateway provider
// that the server is not configured to use.
var ErrUnsupportedProvider = errors.New("requested payment provider is not supported")

// defaultCurrency is the currency used for gateway transactions in the MVP.
const defaultCurrency = "IDR"

// TenantPortalService implements the tenant portal use cases (Module 08): the
// read views (my room, bills, payments) and the Pay Now checkout flow. Every
// method receives tenantID and ownerID derived from the tenant token; nothing
// tenant-scoped is ever taken from the request body (BR-002).
type TenantPortalService interface {
	MyRoom(ctx context.Context, tenantID, ownerID string) (*model.TenantRoomView, error)
	ListBills(ctx context.Context, tenantID, ownerID string, f model.TenantListBillsFilter) (*model.ListBillsResult, error)
	GetBill(ctx context.Context, billID, tenantID, ownerID string) (*model.Bill, error)
	ListPayments(ctx context.Context, tenantID, ownerID string, f model.TenantListPaymentsFilter) (*model.ListPaymentsResult, error)
	PayBill(ctx context.Context, billID, tenantID, ownerID string, in model.PayBillInput) (*model.PayBillResult, error)
}

type tenantPortalService struct {
	repo      repository.TenantPortalRepository
	gateway   repository.GatewayRepository
	provider  gateway.Provider
	returnURL string
	now       func() time.Time
}

// NewTenantPortalService wires a TenantPortalService to its repositories and the
// configured payment gateway provider.
func NewTenantPortalService(repo repository.TenantPortalRepository, gw repository.GatewayRepository, provider gateway.Provider, returnURL string) TenantPortalService {
	return &tenantPortalService{
		repo:      repo,
		gateway:   gw,
		provider:  provider,
		returnURL: returnURL,
		now:       time.Now,
	}
}

func (s *tenantPortalService) MyRoom(ctx context.Context, tenantID, ownerID string) (*model.TenantRoomView, error) {
	return s.repo.MyRoom(ctx, tenantID, ownerID)
}

func (s *tenantPortalService) ListBills(ctx context.Context, tenantID, ownerID string, f model.TenantListBillsFilter) (*model.ListBillsResult, error) {
	return s.repo.ListBills(ctx, tenantID, ownerID, f)
}

func (s *tenantPortalService) GetBill(ctx context.Context, billID, tenantID, ownerID string) (*model.Bill, error) {
	return s.repo.GetBill(ctx, billID, tenantID, ownerID)
}

func (s *tenantPortalService) ListPayments(ctx context.Context, tenantID, ownerID string, f model.TenantListPaymentsFilter) (*model.ListPaymentsResult, error) {
	return s.repo.ListPayments(ctx, tenantID, ownerID, f)
}

// PayBill opens a payment gateway checkout for the tenant's own bill and flips
// the bill to gateway_pending, all in one transaction. A bill may only be paid
// when it is unpaid, overdue, or gateway_pending with no live checkout (BR-020).
// If a still-valid pending checkout already exists, it is reused rather than
// duplicated (BR-021). The gateway amount is always the bill amount (BR-022); it
// is never taken from the request.
func (s *tenantPortalService) PayBill(ctx context.Context, billID, tenantID, ownerID string, in model.PayBillInput) (result *model.PayBillResult, err error) {
	if in.Provider != "" && in.Provider != s.provider.Name() {
		return nil, ErrUnsupportedProvider
	}

	tx, err := s.gateway.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	bill, err := s.gateway.BillForUpdate(ctx, tx, billID, tenantID, ownerID)
	if err != nil {
		return nil, err
	}

	switch bill.Status {
	case "paid":
		return nil, ErrBillAlreadyPaid
	case "unpaid", "overdue":
		// Eligible to create a fresh checkout.
	case "gateway_pending":
		// Reuse a still-valid checkout instead of creating a duplicate (BR-021).
		existing, lookupErr := s.gateway.ActivePendingTransaction(ctx, tx, bill.ID, ownerID, s.now().UTC())
		if lookupErr == nil {
			if err = tx.Commit(ctx); err != nil {
				return nil, fmt.Errorf("commit reuse checkout: %w", err)
			}
			return gatewayTxToResult(existing), nil
		}
		if !errors.Is(lookupErr, repository.ErrNotFound) {
			return nil, lookupErr
		}
		// No live checkout (expired): fall through and create a new one.
	default:
		return nil, ErrBillNotPayable
	}

	gt, err := s.createCheckout(ctx, tx, bill, ownerID)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit checkout: %w", err)
	}

	return gatewayTxToResult(gt), nil
}

// createCheckout opens a provider checkout for the bill, stores the resulting
// pending transaction (with its raw response for audit), and flips the bill to
// gateway_pending — all within tx, so the caller controls the commit. The
// gateway amount is always the bill amount (BR-022).
func (s *tenantPortalService) createCheckout(ctx context.Context, tx pgx.Tx, bill *model.Bill, ownerID string) (*model.GatewayTransaction, error) {
	orderID, err := newOrderID()
	if err != nil {
		return nil, err
	}

	checkout, err := s.provider.CreateCheckout(ctx, gateway.CheckoutInput{
		OrderID:    orderID,
		BillNumber: bill.BillNumber,
		BillID:     bill.ID,
		TenantID:   bill.TenantID,
		Amount:     bill.Amount,
		Currency:   defaultCurrency,
		ReturnURL:  s.returnURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create checkout: %w", err)
	}

	expiresAt := checkout.ExpiresAt
	gt, err := s.gateway.InsertTransaction(ctx, tx, model.GatewayTransaction{
		OwnerID:               ownerID,
		BillID:                bill.ID,
		TenantID:              bill.TenantID,
		Provider:              checkout.Provider,
		ExternalTransactionID: optionalStr(checkout.ExternalTransactionID),
		ExternalOrderID:       checkout.ExternalOrderID,
		CheckoutURL:           optionalStr(checkout.CheckoutURL),
		Amount:                bill.Amount,
		Currency:              defaultCurrency,
		Status:                "pending",
		ExpiresAt:             &expiresAt,
	}, checkout.RawResponse)
	if err != nil {
		return nil, err
	}

	// BR-023: opening a checkout moves the bill to gateway_pending.
	if err = s.gateway.SetBillGatewayPending(ctx, tx, bill.ID, ownerID); err != nil {
		return nil, err
	}
	return gt, nil
}

// gatewayTxToResult shapes a stored transaction into the Pay Now API response.
func gatewayTxToResult(gt *model.GatewayTransaction) *model.PayBillResult {
	var url string
	if gt.CheckoutURL != nil {
		url = *gt.CheckoutURL
	}
	return &model.PayBillResult{
		GatewayTransactionID: gt.ID,
		Provider:             gt.Provider,
		CheckoutURL:          url,
		Status:               gt.Status,
		ExpiresAt:            gt.ExpiresAt,
	}
}

// newOrderID returns a unique external order reference for a checkout attempt.
func newOrderID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate order id: %w", err)
	}
	return "ORD-" + hex.EncodeToString(buf), nil
}

// optionalStr returns a pointer to s, or nil when s is empty, so empty optional
// columns persist as NULL rather than "".
func optionalStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

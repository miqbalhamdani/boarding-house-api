package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iqbal-hamdani/go-backend/internal/gateway"
	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// --- stubs ---

// stubPortalRepo records the scope it was queried with so isolation tests can
// assert the service always forwards the token-derived tenant_id and owner_id.
type stubPortalRepo struct {
	gotTenantID string
	gotOwnerID  string
	gotBillID   string

	room     *model.TenantRoomView
	roomErr  error
	bills    *model.ListBillsResult
	bill     *model.Bill
	billErr  error
	payments *model.ListPaymentsResult
}

func (s *stubPortalRepo) MyRoom(_ context.Context, tenantID, ownerID string) (*model.TenantRoomView, error) {
	s.gotTenantID, s.gotOwnerID = tenantID, ownerID
	return s.room, s.roomErr
}

func (s *stubPortalRepo) ListBills(_ context.Context, tenantID, ownerID string, _ model.TenantListBillsFilter) (*model.ListBillsResult, error) {
	s.gotTenantID, s.gotOwnerID = tenantID, ownerID
	return s.bills, nil
}

func (s *stubPortalRepo) GetBill(_ context.Context, billID, tenantID, ownerID string) (*model.Bill, error) {
	s.gotBillID, s.gotTenantID, s.gotOwnerID = billID, tenantID, ownerID
	return s.bill, s.billErr
}

func (s *stubPortalRepo) ListPayments(_ context.Context, tenantID, ownerID string, _ model.TenantListPaymentsFilter) (*model.ListPaymentsResult, error) {
	s.gotTenantID, s.gotOwnerID = tenantID, ownerID
	return s.payments, nil
}

// stubGatewayRepo models the locked bill, the active-pending lookup, and records
// which writes the service issued during the Pay Now flow.
type stubGatewayRepo struct {
	bill    *model.Bill
	billErr error

	gotBillID   string
	gotTenantID string
	gotOwnerID  string

	activePending    *model.GatewayTransaction
	activePendingErr error

	inserted     *model.GatewayTransaction
	insertErr    error
	billPending  bool
	insertedRaw  []byte
	insertCalled bool
}

func (s *stubGatewayRepo) BeginTx(context.Context) (pgx.Tx, error) { return fakeTx{}, nil }

func (s *stubGatewayRepo) BillForUpdate(_ context.Context, _ pgx.Tx, billID, tenantID, ownerID string) (*model.Bill, error) {
	s.gotBillID, s.gotTenantID, s.gotOwnerID = billID, tenantID, ownerID
	if s.billErr != nil {
		return nil, s.billErr
	}
	return s.bill, nil
}

func (s *stubGatewayRepo) ActivePendingTransaction(_ context.Context, _ pgx.Tx, _, _ string, _ time.Time) (*model.GatewayTransaction, error) {
	if s.activePendingErr != nil {
		return nil, s.activePendingErr
	}
	return s.activePending, nil
}

func (s *stubGatewayRepo) InsertTransaction(_ context.Context, _ pgx.Tx, gt model.GatewayTransaction, raw []byte) (*model.GatewayTransaction, error) {
	s.insertCalled = true
	s.insertedRaw = raw
	if s.insertErr != nil {
		return nil, s.insertErr
	}
	gt.ID = "gt-1"
	s.inserted = &gt
	return &gt, nil
}

func (s *stubGatewayRepo) SetBillGatewayPending(_ context.Context, _ pgx.Tx, _, _ string) error {
	s.billPending = true
	return nil
}

// stubProvider is an in-memory gateway provider that records the checkout input.
type stubProvider struct {
	name      string
	result    *gateway.CheckoutResult
	err       error
	lastInput gateway.CheckoutInput
	called    bool
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) CreateCheckout(_ context.Context, in gateway.CheckoutInput) (*gateway.CheckoutResult, error) {
	p.called = true
	p.lastInput = in
	if p.err != nil {
		return nil, p.err
	}
	return p.result, nil
}

// --- helpers ---

func unpaidPortalBill() *model.Bill {
	return &model.Bill{
		ID:               "bill-1",
		OwnerID:          "owner-1",
		TenantID:         "tenant-1",
		RoomID:           "room-1",
		RoomAssignmentID: "assign-1",
		BillNumber:       "BILL-2026-07-001",
		Amount:           2000000,
		Status:           "unpaid",
	}
}

func okProvider() *stubProvider {
	expires := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	url := "https://sandbox.pay.local/checkout/ORD-abc"
	return &stubProvider{
		name: "sandbox",
		result: &gateway.CheckoutResult{
			Provider:              "sandbox",
			ExternalOrderID:       "ORD-abc",
			ExternalTransactionID: "sbx_ORD-abc",
			CheckoutURL:           url,
			ExpiresAt:             expires,
			RawResponse:           []byte(`{"order_id":"ORD-abc"}`),
		},
	}
}

func newPortalSvc(repo *stubPortalRepo, gw *stubGatewayRepo, p gateway.Provider) TenantPortalService {
	return NewTenantPortalService(repo, gw, p, "https://app.example.com/return")
}

// Ensure stubs satisfy the interfaces.
var (
	_ repository.TenantPortalRepository = (*stubPortalRepo)(nil)
	_ repository.GatewayRepository      = (*stubGatewayRepo)(nil)
	_ gateway.Provider                  = (*stubProvider)(nil)
)

// --- read isolation tests (BR-002) ---

func TestMyRoom_DerivesTenantAndOwnerFromArgs(t *testing.T) {
	repo := &stubPortalRepo{room: &model.TenantRoomView{RoomAssignmentID: "a-1"}}
	svc := newPortalSvc(repo, &stubGatewayRepo{}, okProvider())

	if _, err := svc.MyRoom(context.Background(), "tenant-9", "owner-9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotTenantID != "tenant-9" || repo.gotOwnerID != "owner-9" {
		t.Fatalf("scope not forwarded: tenant=%q owner=%q", repo.gotTenantID, repo.gotOwnerID)
	}
}

func TestListBills_DerivesTenantAndOwnerFromArgs(t *testing.T) {
	repo := &stubPortalRepo{bills: &model.ListBillsResult{Bills: []*model.Bill{}}}
	svc := newPortalSvc(repo, &stubGatewayRepo{}, okProvider())

	if _, err := svc.ListBills(context.Background(), "tenant-9", "owner-9", model.TenantListBillsFilter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotTenantID != "tenant-9" || repo.gotOwnerID != "owner-9" {
		t.Fatalf("scope not forwarded: tenant=%q owner=%q", repo.gotTenantID, repo.gotOwnerID)
	}
}

func TestGetBill_ForwardsBillTenantOwner(t *testing.T) {
	repo := &stubPortalRepo{bill: unpaidPortalBill()}
	svc := newPortalSvc(repo, &stubGatewayRepo{}, okProvider())

	if _, err := svc.GetBill(context.Background(), "bill-1", "tenant-9", "owner-9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotBillID != "bill-1" || repo.gotTenantID != "tenant-9" || repo.gotOwnerID != "owner-9" {
		t.Fatalf("scope not forwarded: bill=%q tenant=%q owner=%q", repo.gotBillID, repo.gotTenantID, repo.gotOwnerID)
	}
}

func TestGetBill_CrossTenantReturnsNotFound(t *testing.T) {
	// The repo filters by tenant_id+owner_id, so another tenant's bill simply is
	// not found — the service surfaces that verbatim (BR-002).
	repo := &stubPortalRepo{billErr: repository.ErrNotFound}
	svc := newPortalSvc(repo, &stubGatewayRepo{}, okProvider())

	if _, err := svc.GetBill(context.Background(), "bill-1", "tenant-x", "owner-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListPayments_DerivesTenantAndOwnerFromArgs(t *testing.T) {
	repo := &stubPortalRepo{payments: &model.ListPaymentsResult{Payments: []*model.Payment{}}}
	svc := newPortalSvc(repo, &stubGatewayRepo{}, okProvider())

	if _, err := svc.ListPayments(context.Background(), "tenant-9", "owner-9", model.TenantListPaymentsFilter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotTenantID != "tenant-9" || repo.gotOwnerID != "owner-9" {
		t.Fatalf("scope not forwarded: tenant=%q owner=%q", repo.gotTenantID, repo.gotOwnerID)
	}
}

// --- PayBill tests ---

func TestPayBill_UnpaidCreatesCheckout(t *testing.T) {
	gw := &stubGatewayRepo{bill: unpaidPortalBill()}
	prov := okProvider()
	svc := newPortalSvc(&stubPortalRepo{}, gw, prov)

	res, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", model.PayBillInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gw.insertCalled || gw.inserted == nil {
		t.Fatal("a gateway transaction should be inserted")
	}
	if !gw.billPending {
		t.Fatal("bill should be moved to gateway_pending (BR-023)")
	}
	if res.CheckoutURL != "https://sandbox.pay.local/checkout/ORD-abc" || res.Status != "pending" {
		t.Fatalf("unexpected result: %+v", res)
	}
	// BR-022: gateway amount equals bill amount and is never taken from the request.
	if prov.lastInput.Amount != 2000000 {
		t.Fatalf("checkout amount must equal bill amount, got %d", prov.lastInput.Amount)
	}
	// Owner isolation: the locked bill was scoped to the token tenant+owner.
	if gw.gotTenantID != "tenant-1" || gw.gotOwnerID != "owner-1" {
		t.Fatalf("bill not scoped to token: tenant=%q owner=%q", gw.gotTenantID, gw.gotOwnerID)
	}
	if gw.inserted.Status != "pending" || gw.inserted.Amount != 2000000 {
		t.Fatalf("stored transaction wrong: %+v", gw.inserted)
	}
}

func TestPayBill_OverdueEligible(t *testing.T) {
	bill := unpaidPortalBill()
	bill.Status = "overdue"
	gw := &stubGatewayRepo{bill: bill}
	svc := newPortalSvc(&stubPortalRepo{}, gw, okProvider())

	if _, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", model.PayBillInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gw.insertCalled {
		t.Fatal("overdue bill should be eligible for checkout (BR-020)")
	}
}

func TestPayBill_AlreadyPaidRejected(t *testing.T) {
	bill := unpaidPortalBill()
	bill.Status = "paid"
	gw := &stubGatewayRepo{bill: bill}
	prov := okProvider()
	svc := newPortalSvc(&stubPortalRepo{}, gw, prov)

	if _, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", model.PayBillInput{}); !errors.Is(err, ErrBillAlreadyPaid) {
		t.Fatalf("want ErrBillAlreadyPaid, got %v", err)
	}
	if gw.insertCalled || prov.called {
		t.Fatal("no checkout should be created for a paid bill")
	}
}

func TestPayBill_CancelledRejected(t *testing.T) {
	bill := unpaidPortalBill()
	bill.Status = "cancelled"
	gw := &stubGatewayRepo{bill: bill}
	svc := newPortalSvc(&stubPortalRepo{}, gw, okProvider())

	if _, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", model.PayBillInput{}); !errors.Is(err, ErrBillNotPayable) {
		t.Fatalf("want ErrBillNotPayable, got %v", err)
	}
	if gw.insertCalled {
		t.Fatal("no checkout should be created for a cancelled bill")
	}
}

func TestPayBill_GatewayPendingReusesActiveCheckout(t *testing.T) {
	// BR-021: a still-valid pending checkout is reused, not duplicated.
	bill := unpaidPortalBill()
	bill.Status = "gateway_pending"
	url := "https://sandbox.pay.local/checkout/ORD-existing"
	expires := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	gw := &stubGatewayRepo{
		bill: bill,
		activePending: &model.GatewayTransaction{
			ID:          "gt-existing",
			Provider:    "sandbox",
			CheckoutURL: &url,
			Status:      "pending",
			ExpiresAt:   &expires,
		},
	}
	prov := okProvider()
	svc := newPortalSvc(&stubPortalRepo{}, gw, prov)

	res, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", model.PayBillInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw.insertCalled || prov.called {
		t.Fatal("existing valid checkout should be reused, not recreated")
	}
	if res.GatewayTransactionID != "gt-existing" || res.CheckoutURL != url {
		t.Fatalf("should return the existing checkout, got %+v", res)
	}
}

func TestPayBill_GatewayPendingExpiredCreatesNew(t *testing.T) {
	bill := unpaidPortalBill()
	bill.Status = "gateway_pending"
	gw := &stubGatewayRepo{bill: bill, activePendingErr: repository.ErrNotFound}
	prov := okProvider()
	svc := newPortalSvc(&stubPortalRepo{}, gw, prov)

	res, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", model.PayBillInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gw.insertCalled || !prov.called {
		t.Fatal("expired gateway_pending bill should create a fresh checkout (BR-027)")
	}
	if res.GatewayTransactionID != "gt-1" {
		t.Fatalf("expected new transaction, got %+v", res)
	}
}

func TestPayBill_UnsupportedProviderRejected(t *testing.T) {
	gw := &stubGatewayRepo{bill: unpaidPortalBill()}
	svc := newPortalSvc(&stubPortalRepo{}, gw, okProvider()) // configured provider: sandbox

	in := model.PayBillInput{Provider: "midtrans"}
	if _, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", in); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("want ErrUnsupportedProvider, got %v", err)
	}
	if gw.insertCalled {
		t.Fatal("no checkout should be created for an unsupported provider")
	}
}

func TestPayBill_BillNotFoundIsForwarded(t *testing.T) {
	// Cross-tenant access: BillForUpdate filters by tenant_id+owner_id, so another
	// tenant's bill is not found and PayBill surfaces ErrNotFound (BR-002).
	gw := &stubGatewayRepo{billErr: repository.ErrNotFound}
	svc := newPortalSvc(&stubPortalRepo{}, gw, okProvider())

	if _, err := svc.PayBill(context.Background(), "bill-1", "tenant-x", "owner-1", model.PayBillInput{}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPayBill_DuplicateTransactionRejected(t *testing.T) {
	gw := &stubGatewayRepo{bill: unpaidPortalBill(), insertErr: repository.ErrDuplicateGatewayTransaction}
	svc := newPortalSvc(&stubPortalRepo{}, gw, okProvider())

	if _, err := svc.PayBill(context.Background(), "bill-1", "tenant-1", "owner-1", model.PayBillInput{}); !errors.Is(err, repository.ErrDuplicateGatewayTransaction) {
		t.Fatalf("want ErrDuplicateGatewayTransaction, got %v", err)
	}
}

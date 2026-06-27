package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// stubPaymentRepo is an in-memory PaymentRepository for unit tests. It models
// the locked bill plus the assignment status, records which activation updates
// the service issued, and can simulate the unique(bill_id) idempotency guard.
type stubPaymentRepo struct {
	bill       *model.Bill
	billErr    error
	assignment string // assignment status returned by AssignmentStatusForUpdate

	insertErr error
	inserted  *model.Payment

	billPaidAt   *time.Time
	billPaid     bool
	activatedA   bool
	activatedT   bool
	occupiedRoom bool

	listResult *model.ListPaymentsResult
	listOwner  string
	getResult  *model.Payment
	getErr     error
}

func (s *stubPaymentRepo) List(_ context.Context, ownerID string, _ model.ListPaymentsFilter) (*model.ListPaymentsResult, error) {
	s.listOwner = ownerID
	return s.listResult, nil
}

func (s *stubPaymentRepo) GetByID(_ context.Context, _, _ string) (*model.Payment, error) {
	return s.getResult, s.getErr
}

func (s *stubPaymentRepo) BeginTx(context.Context) (pgx.Tx, error) { return fakeTx{}, nil }

func (s *stubPaymentRepo) BillForUpdate(_ context.Context, _ pgx.Tx, _, _ string) (*model.Bill, error) {
	if s.billErr != nil {
		return nil, s.billErr
	}
	return s.bill, nil
}

func (s *stubPaymentRepo) InsertPayment(_ context.Context, _ pgx.Tx, p model.Payment) (*model.Payment, error) {
	if s.insertErr != nil {
		return nil, s.insertErr
	}
	p.ID = "payment-1"
	s.inserted = &p
	return &p, nil
}

func (s *stubPaymentRepo) MarkBillPaid(_ context.Context, _ pgx.Tx, _, _ string, paidAt time.Time) error {
	s.billPaid = true
	s.billPaidAt = &paidAt
	return nil
}

func (s *stubPaymentRepo) AssignmentStatusForUpdate(_ context.Context, _ pgx.Tx, _, _ string) (string, error) {
	return s.assignment, nil
}

func (s *stubPaymentRepo) ActivateAssignment(_ context.Context, _ pgx.Tx, _, _ string) error {
	s.activatedA = true
	return nil
}

func (s *stubPaymentRepo) ActivateTenant(_ context.Context, _ pgx.Tx, _, _ string) error {
	s.activatedT = true
	return nil
}

func (s *stubPaymentRepo) OccupyRoom(_ context.Context, _ pgx.Tx, _, _ string) error {
	s.occupiedRoom = true
	return nil
}

func unpaidBill() *model.Bill {
	return &model.Bill{
		ID:               "bill-1",
		OwnerID:          "owner-1",
		TenantID:         "tenant-1",
		RoomID:           "room-1",
		RoomAssignmentID: "assign-1",
		Amount:           2000000,
		Status:           "unpaid",
	}
}

func validInput() model.RecordManualPaymentInput {
	return model.RecordManualPaymentInput{
		BillID:        "11111111-1111-1111-1111-111111111111",
		Amount:        2000000,
		PaymentMethod: "bank_transfer",
		PaymentDate:   "2026-07-10T10:00:00Z",
	}
}

func TestRecordManual_Success_ActivatesPendingTenancy(t *testing.T) {
	// First payment for a pending assignment activates tenant, assignment, room
	// (BR-006, BR-008) and the bill becomes paid (BR-029).
	repo := &stubPaymentRepo{bill: unpaidBill(), assignment: "pending_payment"}
	svc := NewPaymentService(repo)

	p, err := svc.RecordManual(context.Background(), "owner-1", validInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.PaymentSource != "manual" || p.Amount != 2000000 {
		t.Fatalf("unexpected payment: %+v", p)
	}
	if p.OwnerID != "owner-1" || p.TenantID != "tenant-1" || p.RoomID != "room-1" || p.BillID != "bill-1" {
		t.Fatalf("payment linkage derived from bill, got: %+v", p)
	}
	if !repo.billPaid {
		t.Fatal("bill should be marked paid")
	}
	if !repo.activatedA || !repo.activatedT || !repo.occupiedRoom {
		t.Fatalf("first payment must activate assignment/tenant/room: %+v", repo)
	}
	if repo.billPaidAt == nil || !repo.billPaidAt.Equal(time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("paid_at should equal payment_date, got %v", repo.billPaidAt)
	}
}

func TestRecordManual_Success_ActiveAssignmentNotReactivated(t *testing.T) {
	// A later monthly payment leaves an already-active tenancy untouched.
	repo := &stubPaymentRepo{bill: unpaidBill(), assignment: "active"}
	svc := NewPaymentService(repo)

	if _, err := svc.RecordManual(context.Background(), "owner-1", validInput()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.billPaid {
		t.Fatal("bill should be marked paid")
	}
	if repo.activatedA || repo.activatedT || repo.occupiedRoom {
		t.Fatalf("active assignment must not be re-activated: %+v", repo)
	}
}

func TestRecordManual_PartialAmountRejected(t *testing.T) {
	repo := &stubPaymentRepo{bill: unpaidBill(), assignment: "pending_payment"}
	svc := NewPaymentService(repo)

	in := validInput()
	in.Amount = 1000000 // less than the 2,000,000 bill
	if _, err := svc.RecordManual(context.Background(), "owner-1", in); !errors.Is(err, ErrPaymentAmountMismatch) {
		t.Fatalf("want ErrPaymentAmountMismatch, got %v", err)
	}
	if repo.inserted != nil || repo.billPaid {
		t.Fatal("no payment should be recorded on amount mismatch")
	}
}

func TestRecordManual_OverpaymentRejected(t *testing.T) {
	repo := &stubPaymentRepo{bill: unpaidBill(), assignment: "pending_payment"}
	svc := NewPaymentService(repo)

	in := validInput()
	in.Amount = 3000000
	if _, err := svc.RecordManual(context.Background(), "owner-1", in); !errors.Is(err, ErrPaymentAmountMismatch) {
		t.Fatalf("want ErrPaymentAmountMismatch, got %v", err)
	}
}

func TestRecordManual_AlreadyPaidRejected(t *testing.T) {
	bill := unpaidBill()
	bill.Status = "paid"
	repo := &stubPaymentRepo{bill: bill}
	svc := NewPaymentService(repo)

	if _, err := svc.RecordManual(context.Background(), "owner-1", validInput()); !errors.Is(err, ErrBillAlreadyPaid) {
		t.Fatalf("want ErrBillAlreadyPaid, got %v", err)
	}
	if repo.inserted != nil {
		t.Fatal("no payment should be recorded for an already-paid bill")
	}
}

func TestRecordManual_CancelledBillRejected(t *testing.T) {
	bill := unpaidBill()
	bill.Status = "cancelled"
	repo := &stubPaymentRepo{bill: bill}
	svc := NewPaymentService(repo)

	if _, err := svc.RecordManual(context.Background(), "owner-1", validInput()); !errors.Is(err, ErrBillNotPayable) {
		t.Fatalf("want ErrBillNotPayable, got %v", err)
	}
}

func TestRecordManual_DuplicatePaymentRejected(t *testing.T) {
	// BR-030: unique(bill_id) rejects a second successful payment even when a
	// concurrent caller passed the status check first.
	repo := &stubPaymentRepo{bill: unpaidBill(), assignment: "active", insertErr: repository.ErrDuplicatePayment}
	svc := NewPaymentService(repo)

	if _, err := svc.RecordManual(context.Background(), "owner-1", validInput()); !errors.Is(err, ErrDuplicatePayment) {
		t.Fatalf("want ErrDuplicatePayment, got %v", err)
	}
}

func TestRecordManual_InvalidPaymentMethodRejected(t *testing.T) {
	repo := &stubPaymentRepo{bill: unpaidBill()}
	svc := NewPaymentService(repo)

	in := validInput()
	in.PaymentMethod = "bitcoin"
	if _, err := svc.RecordManual(context.Background(), "owner-1", in); !errors.Is(err, ErrInvalidPaymentMethod) {
		t.Fatalf("want ErrInvalidPaymentMethod, got %v", err)
	}
}

func TestRecordManual_InvalidPaymentDateRejected(t *testing.T) {
	repo := &stubPaymentRepo{bill: unpaidBill()}
	svc := NewPaymentService(repo)

	in := validInput()
	in.PaymentDate = "10-07-2026"
	if _, err := svc.RecordManual(context.Background(), "owner-1", in); !errors.Is(err, ErrInvalidPaymentDate) {
		t.Fatalf("want ErrInvalidPaymentDate, got %v", err)
	}
}

func TestRecordManual_DefaultsPaymentDateToNow(t *testing.T) {
	repo := &stubPaymentRepo{bill: unpaidBill(), assignment: "active"}
	fixed := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	svc := &paymentService{repo: repo, now: func() time.Time { return fixed }}

	in := validInput()
	in.PaymentDate = ""
	p, err := svc.RecordManual(context.Background(), "owner-1", in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PaymentDate.Equal(fixed) {
		t.Fatalf("payment_date should default to now, got %v", p.PaymentDate)
	}
}

func TestRecordManual_BillNotFound(t *testing.T) {
	repo := &stubPaymentRepo{billErr: repository.ErrNotFound}
	svc := NewPaymentService(repo)

	if _, err := svc.RecordManual(context.Background(), "owner-1", validInput()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPaymentList_DerivesOwnerFromArgument(t *testing.T) {
	// Owner isolation (BR-001): the service forwards the authenticated owner_id.
	repo := &stubPaymentRepo{listResult: &model.ListPaymentsResult{Payments: []*model.Payment{}}}
	svc := NewPaymentService(repo)

	if _, err := svc.List(context.Background(), "owner-42", model.ListPaymentsFilter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listOwner != "owner-42" {
		t.Fatalf("List used wrong owner_id: %q", repo.listOwner)
	}
}

// Ensure the stub satisfies the repository interface.
var _ repository.PaymentRepository = (*stubPaymentRepo)(nil)

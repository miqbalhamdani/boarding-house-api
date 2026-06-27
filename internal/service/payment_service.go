package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// Domain errors for the payment-tracking flow. Handlers map these to HTTP
// responses; the repository's ErrDuplicatePayment is re-exported here so the
// handler can branch on a single package's error set.
var (
	ErrInvalidPaymentDate    = errors.New("payment_date must be a valid RFC3339 timestamp")
	ErrInvalidPaymentMethod  = errors.New("payment_method is not supported")
	ErrBillAlreadyPaid       = errors.New("bill is already paid")
	ErrBillNotPayable        = errors.New("bill cannot be paid in its current status")
	ErrPaymentAmountMismatch = errors.New("payment amount must equal the bill amount")
	ErrDuplicatePayment      = repository.ErrDuplicatePayment
)

// allowedPaymentMethods bounds the payment_method values accepted for a manual
// payment (see database-schema.md).
var allowedPaymentMethods = map[string]bool{
	"cash":            true,
	"bank_transfer":   true,
	"e_wallet":        true,
	"virtual_account": true,
	"credit_card":     true,
	"qris":            true,
	"other":           true,
}

// PaymentService implements the payment-tracking use cases (Module 06).
type PaymentService interface {
	RecordManual(ctx context.Context, ownerID string, in model.RecordManualPaymentInput) (*model.Payment, error)
	List(ctx context.Context, ownerID string, f model.ListPaymentsFilter) (*model.ListPaymentsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Payment, error)
}

type paymentService struct {
	repo repository.PaymentRepository
	now  func() time.Time
}

// NewPaymentService wires a PaymentService to its repository.
func NewPaymentService(repo repository.PaymentRepository) PaymentService {
	return &paymentService{repo: repo, now: time.Now}
}

func (s *paymentService) List(ctx context.Context, ownerID string, f model.ListPaymentsFilter) (*model.ListPaymentsResult, error) {
	return s.repo.List(ctx, ownerID, f)
}

func (s *paymentService) GetByID(ctx context.Context, id, ownerID string) (*model.Payment, error) {
	return s.repo.GetByID(ctx, id, ownerID)
}

// RecordManual records a manual full payment and, in the same transaction,
// flips the bill to paid and — when this is the first payment for a pending
// assignment — activates the tenant, assignment, and room (BR-006, BR-008,
// BR-028, BR-029). The whole operation is atomic (coding-rules: payment
// creation and bill update must happen in one DB transaction).
func (s *paymentService) RecordManual(ctx context.Context, ownerID string, in model.RecordManualPaymentInput) (result *model.Payment, err error) {
	if !allowedPaymentMethods[in.PaymentMethod] {
		return nil, ErrInvalidPaymentMethod
	}

	paymentDate := s.now().UTC()
	if in.PaymentDate != "" {
		parsed, perr := time.Parse(time.RFC3339, in.PaymentDate)
		if perr != nil {
			return nil, ErrInvalidPaymentDate
		}
		paymentDate = parsed.UTC()
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the bill so amount/status checks and the paid transition are
	// race-safe against a concurrent payment attempt on the same bill.
	bill, err := s.repo.BillForUpdate(ctx, tx, in.BillID, ownerID)
	if err != nil {
		return nil, err
	}

	switch bill.Status {
	case "paid":
		return nil, ErrBillAlreadyPaid
	case "cancelled":
		return nil, ErrBillNotPayable
	}

	// BR-019 / BR-031: a bill must be paid in full; the amount must match exactly.
	if in.Amount != bill.Amount {
		return nil, ErrPaymentAmountMismatch
	}

	payment, err := s.repo.InsertPayment(ctx, tx, model.Payment{
		OwnerID:         ownerID,
		BillID:          bill.ID,
		TenantID:        bill.TenantID,
		RoomID:          bill.RoomID,
		Amount:          in.Amount,
		PaymentDate:     paymentDate,
		PaymentMethod:   in.PaymentMethod,
		PaymentSource:   "manual", // BR: manual payment source is `manual`.
		ReferenceNumber: optional(in.ReferenceNumber),
		Notes:           optional(in.Notes),
	})
	if err != nil {
		return nil, err
	}

	if err = s.repo.MarkBillPaid(ctx, tx, bill.ID, ownerID, paymentDate); err != nil {
		return nil, err
	}

	// First-payment activation: only when the assignment is still pending does
	// paying its bill activate the tenancy. Subsequent monthly payments leave
	// the already-active assignment untouched.
	assignmentStatus, err := s.repo.AssignmentStatusForUpdate(ctx, tx, bill.RoomAssignmentID, ownerID)
	if err != nil {
		return nil, err
	}
	if assignmentStatus == "pending_payment" {
		if err = s.repo.ActivateAssignment(ctx, tx, bill.RoomAssignmentID, ownerID); err != nil {
			return nil, err
		}
		if err = s.repo.ActivateTenant(ctx, tx, bill.TenantID, ownerID); err != nil {
			return nil, err
		}
		if err = s.repo.OccupyRoom(ctx, tx, bill.RoomID, ownerID); err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit manual payment: %w", err)
	}

	return payment, nil
}

// optional returns a pointer to s, or nil when s is empty, so empty optional
// text columns persist as NULL rather than "".
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

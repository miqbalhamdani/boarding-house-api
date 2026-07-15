package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/internal/storage"
)

// Domain errors for the manual payment-proof flow. Errors shared with Module 06
// (ErrBillAlreadyPaid, ErrBillNotPayable, ErrPaymentAmountMismatch,
// ErrDuplicatePayment) are reused from payment_service.go, not redefined.
var (
	ErrBillNotSubmittable         = errors.New("bill cannot receive a payment-proof submission in its current status")
	ErrSubmissionAmountMismatch   = errors.New("submitted amount must equal the bill amount")
	ErrProofRequired              = errors.New("proof file is required")
	ErrSubmissionNotPending       = errors.New("submission is not pending review")
	ErrRejectReasonRequired       = errors.New("reason is required")
	ErrDuplicatePendingSubmission = repository.ErrDuplicatePendingSubmission
)

// ManualPaymentService implements the manual payment-proof use cases (Module 10).
type ManualPaymentService interface {
	// Tenant
	Submit(ctx context.Context, tenantID, ownerID, billID string, in model.SubmitManualPaymentInput) (*model.ManualPaymentSubmission, error)
	GetForTenant(ctx context.Context, tenantID, ownerID, billID string) (*model.ManualPaymentSubmission, error)
	Cancel(ctx context.Context, tenantID, ownerID, submissionID string) error

	// Owner
	List(ctx context.Context, ownerID string, f model.ListManualPaymentSubmissionsFilter) (*model.ListManualPaymentSubmissionsResult, error)
	GetDetailForOwner(ctx context.Context, ownerID, submissionID string) (*model.ManualPaymentSubmission, error)
	Approve(ctx context.Context, ownerID, submissionID, reviewerID string, in model.ReviewSubmissionInput) (*model.Payment, error)
	Reject(ctx context.Context, ownerID, submissionID, reviewerID string, in model.RejectSubmissionInput) error
}

type manualPaymentService struct {
	subRepo     repository.ManualPaymentSubmissionRepository
	paymentRepo repository.PaymentRepository
	billRepo    repository.TenantPortalRepository
	store       storage.Store
	presignTTL  time.Duration
	now         func() time.Time
}

// NewManualPaymentService wires the service to its repositories and object store.
// It reuses PaymentRepository so the approval transaction shares the exact
// bill-lock / insert-payment / activation logic used by Module 06, and
// TenantPortalRepository.GetBill so a bill lookup enforces both owner and tenant
// ownership in one query.
func NewManualPaymentService(
	subRepo repository.ManualPaymentSubmissionRepository,
	paymentRepo repository.PaymentRepository,
	billRepo repository.TenantPortalRepository,
	store storage.Store,
	presignTTL time.Duration,
) ManualPaymentService {
	return &manualPaymentService{
		subRepo:     subRepo,
		paymentRepo: paymentRepo,
		billRepo:    billRepo,
		store:       store,
		presignTTL:  presignTTL,
		now:         time.Now,
	}
}

// Submit validates and stores a tenant's payment-proof claim. It never marks
// the bill paid or creates a payment (Module 10 business rules).
func (s *manualPaymentService) Submit(ctx context.Context, tenantID, ownerID, billID string, in model.SubmitManualPaymentInput) (*model.ManualPaymentSubmission, error) {
	if in.SubmittedAmount <= 0 {
		return nil, ErrSubmissionAmountMismatch
	}
	if !allowedPaymentMethods[in.PaymentMethod] {
		return nil, ErrInvalidPaymentMethod
	}
	transferDate, perr := time.Parse(time.RFC3339, in.TransferDate)
	if perr != nil {
		return nil, ErrInvalidPaymentDate
	}
	if len(in.ProofContent) == 0 {
		return nil, ErrProofRequired
	}

	// Validate the proof by content, not filename, before any DB write.
	ext, contentType, verr := storage.DetectAndValidate(in.ProofContent)
	if verr != nil {
		return nil, verr
	}

	// One tenant+owner-scoped lookup enforces both ownership rules (BR-001/002).
	bill, err := s.billRepo.GetBill(ctx, billID, tenantID, ownerID)
	if err != nil {
		return nil, err
	}

	// Only unpaid/overdue bills may receive a submission. A paid bill fails here,
	// so no separate "already has a successful payment" query is needed.
	if bill.Status != "unpaid" && bill.Status != "overdue" {
		return nil, ErrBillNotSubmittable
	}

	// Full amount only; partial/over payments are rejected.
	if in.SubmittedAmount != bill.Amount {
		return nil, ErrSubmissionAmountMismatch
	}

	// Insert first; the partial unique index is the authoritative guard against
	// a duplicate pending submission (avoids a check-then-insert race).
	sub, err := s.subRepo.Create(ctx, model.ManualPaymentSubmission{
		OwnerID:           ownerID,
		BillID:            bill.ID,
		TenantID:          tenantID,
		SubmittedAmount:   in.SubmittedAmount,
		PaymentMethod:     in.PaymentMethod,
		TransferDate:      transferDate.UTC(),
		SenderAccountName: nullable(in.SenderAccountName),
		ReferenceNumber:   nullable(in.ReferenceNumber),
		TenantNotes:       nullable(in.Notes),
	})
	if err != nil {
		return nil, err
	}

	// Upload the proof under a server-generated key. On failure, best-effort
	// remove the orphaned row so the tenant can retry without hitting the
	// pending-submission unique constraint (documented two-phase-commit gap).
	key := storage.ProofObjectKey(ownerID, sub.ID, ext)
	if err := s.store.Put(ctx, key, bytes.NewReader(in.ProofContent), int64(len(in.ProofContent)), contentType); err != nil {
		if delErr := s.subRepo.DeleteByID(ctx, sub.ID, ownerID); delErr != nil {
			slog.Error("cleanup orphaned submission after upload failure", "submission_id", sub.ID, "err", delErr)
		}
		return nil, fmt.Errorf("store proof: %w", err)
	}

	if err := s.subRepo.SetProofURL(ctx, sub.ID, ownerID, key); err != nil {
		return nil, err
	}
	sub.ProofURL = &key
	return sub, nil
}

func (s *manualPaymentService) GetForTenant(ctx context.Context, tenantID, ownerID, billID string) (*model.ManualPaymentSubmission, error) {
	return s.subRepo.GetLatestForBill(ctx, billID, tenantID, ownerID)
}

func (s *manualPaymentService) Cancel(ctx context.Context, tenantID, ownerID, submissionID string) error {
	return s.subRepo.Cancel(ctx, submissionID, tenantID, ownerID)
}

func (s *manualPaymentService) List(ctx context.Context, ownerID string, f model.ListManualPaymentSubmissionsFilter) (*model.ListManualPaymentSubmissionsResult, error) {
	return s.subRepo.List(ctx, ownerID, f)
}

// GetDetailForOwner returns a submission and populates a short-lived presigned
// proof URL. The presigned URL is never logged.
func (s *manualPaymentService) GetDetailForOwner(ctx context.Context, ownerID, submissionID string) (*model.ManualPaymentSubmission, error) {
	sub, err := s.subRepo.GetByIDForOwner(ctx, submissionID, ownerID)
	if err != nil {
		return nil, err
	}
	if sub.ProofURL != nil && *sub.ProofURL != "" {
		url, perr := s.store.PresignGet(ctx, *sub.ProofURL, s.presignTTL)
		if perr != nil {
			return nil, fmt.Errorf("presign proof url: %w", perr)
		}
		sub.ProofViewURL = url
	}
	return sub, nil
}

// Approve verifies and approves a submission in one transaction: it creates a
// manual payment, marks the bill paid, marks the submission approved, and — for
// a first payment on a pending assignment — activates the tenancy. This mirrors
// PaymentService.RecordManual, with a submission lock prepended.
func (s *manualPaymentService) Approve(ctx context.Context, ownerID, submissionID, reviewerID string, in model.ReviewSubmissionInput) (result *model.Payment, err error) {
	tx, err := s.paymentRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sub, err := s.subRepo.SubmissionForUpdate(ctx, tx, submissionID, ownerID)
	if err != nil {
		return nil, err
	}
	if sub.Status != "pending_review" {
		return nil, ErrSubmissionNotPending
	}

	bill, err := s.paymentRepo.BillForUpdate(ctx, tx, sub.BillID, ownerID)
	if err != nil {
		return nil, err
	}
	switch bill.Status {
	case "paid":
		return nil, ErrBillAlreadyPaid
	case "cancelled":
		return nil, ErrBillNotPayable
	}

	// Defensive re-check: the bill amount could have changed after submission.
	if sub.SubmittedAmount != bill.Amount {
		return nil, ErrPaymentAmountMismatch
	}

	reviewedAt := s.now().UTC()

	payment, err := s.paymentRepo.InsertPayment(ctx, tx, model.Payment{
		OwnerID:         ownerID,
		BillID:          bill.ID,
		TenantID:        bill.TenantID,
		RoomID:          bill.RoomID,
		Amount:          sub.SubmittedAmount,
		PaymentDate:     sub.TransferDate,
		PaymentMethod:   sub.PaymentMethod,
		PaymentSource:   "manual",
		ReferenceNumber: sub.ReferenceNumber,
	})
	if err != nil {
		return nil, err
	}

	if err = s.paymentRepo.MarkBillPaid(ctx, tx, bill.ID, ownerID, reviewedAt); err != nil {
		return nil, err
	}

	if err = s.subRepo.MarkApproved(ctx, tx, sub.ID, ownerID, reviewerID, in.ReviewNotes, reviewedAt); err != nil {
		return nil, err
	}

	// First-payment activation, identical to RecordManual.
	assignmentStatus, err := s.paymentRepo.AssignmentStatusForUpdate(ctx, tx, bill.RoomAssignmentID, ownerID)
	if err != nil {
		return nil, err
	}
	if assignmentStatus == "pending_payment" {
		if err = s.paymentRepo.ActivateAssignment(ctx, tx, bill.RoomAssignmentID, ownerID); err != nil {
			return nil, err
		}
		if err = s.paymentRepo.ActivateTenant(ctx, tx, bill.TenantID, ownerID); err != nil {
			return nil, err
		}
		if err = s.paymentRepo.OccupyRoom(ctx, tx, bill.RoomID, ownerID); err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit submission approval: %w", err)
	}
	return payment, nil
}

// Reject marks a pending submission rejected with a required reason. No payment
// is created and the bill status is unchanged.
func (s *manualPaymentService) Reject(ctx context.Context, ownerID, submissionID, reviewerID string, in model.RejectSubmissionInput) error {
	if in.Reason == "" {
		return ErrRejectReasonRequired
	}

	sub, err := s.subRepo.GetByIDForOwner(ctx, submissionID, ownerID)
	if err != nil {
		return err
	}
	if sub.Status != "pending_review" {
		return ErrSubmissionNotPending
	}

	if err := s.subRepo.MarkRejected(ctx, submissionID, ownerID, reviewerID, in.Reason, in.ReviewNotes, s.now().UTC()); err != nil {
		// 0 rows despite the pending check above means a concurrent
		// approve/reject/cancel won the race.
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSubmissionNotPending
		}
		return err
	}
	return nil
}

// nullable returns a pointer to s, or nil when s is empty, so empty optional
// text columns persist as NULL rather than "".
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

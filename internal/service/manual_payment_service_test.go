package service

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/internal/storage"
)

// --- stubs ---------------------------------------------------------------

// stubSubmissionRepo is an in-memory ManualPaymentSubmissionRepository.
type stubSubmissionRepo struct {
	created     *model.ManualPaymentSubmission
	createErr   error
	proofURLSet string
	deleted     bool

	latest    *model.ManualPaymentSubmission
	latestErr error
	cancelErr error
	cancelled bool

	listResult *model.ListManualPaymentSubmissionsResult
	listOwner  string

	detail    *model.ManualPaymentSubmission
	detailErr error

	forUpdate    *model.ManualPaymentSubmission
	forUpdateErr error
	approved     bool
	approvedBy   string
	approvedAt   time.Time

	rejectErr    error
	rejected     bool
	rejectReason string
	rejectedBy   string
}

func (s *stubSubmissionRepo) Create(_ context.Context, in model.ManualPaymentSubmission) (*model.ManualPaymentSubmission, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	in.ID = "sub-1"
	in.Status = "pending_review"
	s.created = &in
	return &in, nil
}

func (s *stubSubmissionRepo) SetProofURL(_ context.Context, _, _, proofURL string) error {
	s.proofURLSet = proofURL
	return nil
}

func (s *stubSubmissionRepo) DeleteByID(_ context.Context, _, _ string) error {
	s.deleted = true
	return nil
}

func (s *stubSubmissionRepo) GetLatestForBill(_ context.Context, _, _, _ string) (*model.ManualPaymentSubmission, error) {
	return s.latest, s.latestErr
}

func (s *stubSubmissionRepo) Cancel(_ context.Context, _, _, _ string) error {
	if s.cancelErr != nil {
		return s.cancelErr
	}
	s.cancelled = true
	return nil
}

func (s *stubSubmissionRepo) List(_ context.Context, ownerID string, _ model.ListManualPaymentSubmissionsFilter) (*model.ListManualPaymentSubmissionsResult, error) {
	s.listOwner = ownerID
	return s.listResult, nil
}

func (s *stubSubmissionRepo) GetByIDForOwner(_ context.Context, _, _ string) (*model.ManualPaymentSubmission, error) {
	return s.detail, s.detailErr
}

func (s *stubSubmissionRepo) SubmissionForUpdate(_ context.Context, _ pgx.Tx, _, _ string) (*model.ManualPaymentSubmission, error) {
	return s.forUpdate, s.forUpdateErr
}

func (s *stubSubmissionRepo) MarkApproved(_ context.Context, _ pgx.Tx, _, _, reviewerID, _ string, reviewedAt time.Time) error {
	s.approved = true
	s.approvedBy = reviewerID
	s.approvedAt = reviewedAt
	return nil
}

func (s *stubSubmissionRepo) MarkRejected(_ context.Context, _, _, reviewerID, reason, _ string, _ time.Time) error {
	if s.rejectErr != nil {
		return s.rejectErr
	}
	s.rejected = true
	s.rejectReason = reason
	s.rejectedBy = reviewerID
	return nil
}

// stubBillReader is a minimal TenantPortalRepository providing only GetBill.
type stubBillReader struct {
	bill    *model.Bill
	billErr error
}

func (s *stubBillReader) MyRoom(context.Context, string, string) (*model.TenantRoomView, error) {
	return nil, nil
}
func (s *stubBillReader) ListBills(context.Context, string, string, model.TenantListBillsFilter) (*model.ListBillsResult, error) {
	return nil, nil
}
func (s *stubBillReader) GetBill(_ context.Context, _, _, _ string) (*model.Bill, error) {
	return s.bill, s.billErr
}
func (s *stubBillReader) ListPayments(context.Context, string, string, model.TenantListPaymentsFilter) (*model.ListPaymentsResult, error) {
	return nil, nil
}

// stubStore is an in-memory storage.Store.
type stubStore struct {
	putErr    error
	put       map[string][]byte
	removed   bool
	presigned string
}

func (s *stubStore) Put(_ context.Context, key string, content io.Reader, _ int64, _ string) error {
	if s.putErr != nil {
		return s.putErr
	}
	if s.put == nil {
		s.put = map[string][]byte{}
	}
	b, _ := io.ReadAll(content)
	s.put[key] = b
	return nil
}

func (s *stubStore) PresignGet(_ context.Context, _ string, _ time.Duration) (string, error) {
	if s.presigned == "" {
		return "https://signed.example/proof", nil
	}
	return s.presigned, nil
}

func (s *stubStore) Remove(_ context.Context, _ string) error {
	s.removed = true
	return nil
}

// --- helpers -------------------------------------------------------------

func newManualSvc(sub *stubSubmissionRepo, pay *stubPaymentRepo, bill *stubBillReader, store *stubStore) *manualPaymentService {
	return &manualPaymentService{
		subRepo:     sub,
		paymentRepo: pay,
		billRepo:    bill,
		store:       store,
		presignTTL:  15 * time.Minute,
		now:         func() time.Time { return time.Date(2026, 7, 15, 7, 0, 0, 0, time.UTC) },
	}
}

func validSubmitInput() model.SubmitManualPaymentInput {
	return model.SubmitManualPaymentInput{
		SubmittedAmount: 2000000,
		PaymentMethod:   "bank_transfer",
		TransferDate:    "2026-07-15T07:17:00Z",
		ProofContent:    []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, // JPEG
	}
}

// --- tenant submit -------------------------------------------------------

func TestSubmit_Success_PendingReview(t *testing.T) {
	sub := &stubSubmissionRepo{}
	store := &stubStore{}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{bill: unpaidBill()}, store)

	out, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", validSubmitInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "pending_review" {
		t.Fatalf("want pending_review, got %q", out.Status)
	}
	if sub.proofURLSet == "" || store.put == nil || len(store.put) != 1 {
		t.Fatalf("proof should be uploaded and object key stored: url=%q put=%v", sub.proofURLSet, store.put)
	}
}

func TestSubmit_RejectsOtherTenantsBill(t *testing.T) {
	// GetBill is tenant+owner scoped: a foreign bill returns ErrNotFound.
	svc := newManualSvc(&stubSubmissionRepo{}, &stubPaymentRepo{}, &stubBillReader{billErr: repository.ErrNotFound}, &stubStore{})
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-x", validSubmitInput()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSubmit_RejectsPaidBill(t *testing.T) {
	bill := unpaidBill()
	bill.Status = "paid"
	svc := newManualSvc(&stubSubmissionRepo{}, &stubPaymentRepo{}, &stubBillReader{bill: bill}, &stubStore{})
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", validSubmitInput()); !errors.Is(err, ErrBillNotSubmittable) {
		t.Fatalf("want ErrBillNotSubmittable, got %v", err)
	}
}

func TestSubmit_RejectsCancelledBill(t *testing.T) {
	bill := unpaidBill()
	bill.Status = "cancelled"
	svc := newManualSvc(&stubSubmissionRepo{}, &stubPaymentRepo{}, &stubBillReader{bill: bill}, &stubStore{})
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", validSubmitInput()); !errors.Is(err, ErrBillNotSubmittable) {
		t.Fatalf("want ErrBillNotSubmittable, got %v", err)
	}
}

func TestSubmit_RejectsPartialAmount(t *testing.T) {
	svc := newManualSvc(&stubSubmissionRepo{}, &stubPaymentRepo{}, &stubBillReader{bill: unpaidBill()}, &stubStore{})
	in := validSubmitInput()
	in.SubmittedAmount = 1000000
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", in); !errors.Is(err, ErrSubmissionAmountMismatch) {
		t.Fatalf("want ErrSubmissionAmountMismatch, got %v", err)
	}
}

func TestSubmit_RejectsDuplicatePendingSubmission(t *testing.T) {
	sub := &stubSubmissionRepo{createErr: repository.ErrDuplicatePendingSubmission}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{bill: unpaidBill()}, &stubStore{})
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", validSubmitInput()); !errors.Is(err, ErrDuplicatePendingSubmission) {
		t.Fatalf("want ErrDuplicatePendingSubmission, got %v", err)
	}
}

func TestSubmit_RejectsInvalidFileType(t *testing.T) {
	svc := newManualSvc(&stubSubmissionRepo{}, &stubPaymentRepo{}, &stubBillReader{bill: unpaidBill()}, &stubStore{})
	in := validSubmitInput()
	in.ProofContent = []byte(`<?xml version="1.0"?><svg></svg>`)
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", in); !errors.Is(err, storage.ErrUnsupportedContentType) {
		t.Fatalf("want ErrUnsupportedContentType, got %v", err)
	}
}

func TestSubmit_RejectsOversizedFile(t *testing.T) {
	svc := newManualSvc(&stubSubmissionRepo{}, &stubPaymentRepo{}, &stubBillReader{bill: unpaidBill()}, &stubStore{})
	in := validSubmitInput()
	in.ProofContent = make([]byte, storage.MaxProofFileSize+1)
	copy(in.ProofContent, []byte{0xFF, 0xD8, 0xFF, 0xE0})
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", in); !errors.Is(err, storage.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestSubmit_MissingProofRejected(t *testing.T) {
	svc := newManualSvc(&stubSubmissionRepo{}, &stubPaymentRepo{}, &stubBillReader{bill: unpaidBill()}, &stubStore{})
	in := validSubmitInput()
	in.ProofContent = nil
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", in); !errors.Is(err, ErrProofRequired) {
		t.Fatalf("want ErrProofRequired, got %v", err)
	}
}

func TestSubmit_UploadFailureCleansUpRow(t *testing.T) {
	sub := &stubSubmissionRepo{}
	store := &stubStore{putErr: errors.New("s3 down")}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{bill: unpaidBill()}, store)
	if _, err := svc.Submit(context.Background(), "tenant-1", "owner-1", "bill-1", validSubmitInput()); err == nil {
		t.Fatal("expected upload error")
	}
	if !sub.deleted {
		t.Fatal("orphaned submission row should be deleted after upload failure")
	}
}

// --- tenant cancel -------------------------------------------------------

func TestCancelSubmission_Success(t *testing.T) {
	sub := &stubSubmissionRepo{}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	if err := svc.Cancel(context.Background(), "tenant-1", "owner-1", "sub-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sub.cancelled {
		t.Fatal("submission should be cancelled")
	}
}

func TestCancelSubmission_RejectsNonPendingOrForeign(t *testing.T) {
	sub := &stubSubmissionRepo{cancelErr: repository.ErrNotFound}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	if err := svc.Cancel(context.Background(), "tenant-1", "owner-1", "sub-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// --- owner list / detail -------------------------------------------------

func TestList_ScopedToOwner(t *testing.T) {
	sub := &stubSubmissionRepo{listResult: &model.ListManualPaymentSubmissionsResult{}}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	if _, err := svc.List(context.Background(), "owner-42", model.ListManualPaymentSubmissionsFilter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.listOwner != "owner-42" {
		t.Fatalf("List used wrong owner_id: %q", sub.listOwner)
	}
}

func TestGetDetail_PresignsProof(t *testing.T) {
	key := "owners/owner-1/payment-proofs/sub-1/proof.jpg"
	sub := &stubSubmissionRepo{detail: &model.ManualPaymentSubmission{ID: "sub-1", ProofURL: &key}}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	out, err := svc.GetDetailForOwner(context.Background(), "owner-1", "sub-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ProofViewURL == "" {
		t.Fatal("detail should include a presigned proof view URL")
	}
}

func TestGetDetail_CannotAccessOtherOwner(t *testing.T) {
	sub := &stubSubmissionRepo{detailErr: repository.ErrNotFound}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	if _, err := svc.GetDetailForOwner(context.Background(), "owner-1", "sub-x"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// --- owner approve -------------------------------------------------------

func pendingSubmission() *model.ManualPaymentSubmission {
	return &model.ManualPaymentSubmission{
		ID:              "sub-1",
		OwnerID:         "owner-1",
		BillID:          "bill-1",
		TenantID:        "tenant-1",
		SubmittedAmount: 2000000,
		PaymentMethod:   "bank_transfer",
		TransferDate:    time.Date(2026, 7, 15, 7, 17, 0, 0, time.UTC),
		Status:          "pending_review",
	}
}

func TestApprove_Success_CreatesPaymentMarksBillPaid(t *testing.T) {
	sub := &stubSubmissionRepo{forUpdate: pendingSubmission()}
	pay := &stubPaymentRepo{bill: unpaidBill(), assignment: "active"}
	svc := newManualSvc(sub, pay, &stubBillReader{}, &stubStore{})

	p, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.PaymentSource != "manual" || p.Amount != 2000000 {
		t.Fatalf("unexpected payment: %+v", p)
	}
	if !pay.billPaid {
		t.Fatal("bill should be marked paid")
	}
	if !sub.approved {
		t.Fatal("submission should be marked approved")
	}
}

func TestApprove_Success_StoresReviewerAndTimestamp(t *testing.T) {
	sub := &stubSubmissionRepo{forUpdate: pendingSubmission()}
	pay := &stubPaymentRepo{bill: unpaidBill(), assignment: "active"}
	svc := newManualSvc(sub, pay, &stubBillReader{}, &stubStore{})

	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-9", model.ReviewSubmissionInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.approvedBy != "reviewer-9" {
		t.Fatalf("reviewer not stored: %q", sub.approvedBy)
	}
	if !sub.approvedAt.Equal(time.Date(2026, 7, 15, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("reviewed_at not stored: %v", sub.approvedAt)
	}
}

func TestApprove_Success_ActivatesPendingTenancy(t *testing.T) {
	sub := &stubSubmissionRepo{forUpdate: pendingSubmission()}
	pay := &stubPaymentRepo{bill: unpaidBill(), assignment: "pending_payment"}
	svc := newManualSvc(sub, pay, &stubBillReader{}, &stubStore{})

	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pay.activatedA || !pay.activatedT || !pay.occupiedRoom {
		t.Fatalf("first payment must activate assignment/tenant/room: %+v", pay)
	}
}

func TestApprove_Success_ActiveAssignmentNotReactivated(t *testing.T) {
	sub := &stubSubmissionRepo{forUpdate: pendingSubmission()}
	pay := &stubPaymentRepo{bill: unpaidBill(), assignment: "active"}
	svc := newManualSvc(sub, pay, &stubBillReader{}, &stubStore{})

	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pay.activatedA || pay.activatedT || pay.occupiedRoom {
		t.Fatalf("active assignment must not be reactivated: %+v", pay)
	}
}

func TestApprove_RejectsAlreadyApproved(t *testing.T) {
	s := pendingSubmission()
	s.Status = "approved"
	svc := newManualSvc(&stubSubmissionRepo{forUpdate: s}, &stubPaymentRepo{bill: unpaidBill()}, &stubBillReader{}, &stubStore{})
	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{}); !errors.Is(err, ErrSubmissionNotPending) {
		t.Fatalf("want ErrSubmissionNotPending, got %v", err)
	}
}

func TestApprove_RejectsAlreadyRejected(t *testing.T) {
	s := pendingSubmission()
	s.Status = "rejected"
	svc := newManualSvc(&stubSubmissionRepo{forUpdate: s}, &stubPaymentRepo{bill: unpaidBill()}, &stubBillReader{}, &stubStore{})
	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{}); !errors.Is(err, ErrSubmissionNotPending) {
		t.Fatalf("want ErrSubmissionNotPending, got %v", err)
	}
}

func TestApprove_RejectsAmountMismatch(t *testing.T) {
	s := pendingSubmission()
	s.SubmittedAmount = 999 // bill amount is 2,000,000
	svc := newManualSvc(&stubSubmissionRepo{forUpdate: s}, &stubPaymentRepo{bill: unpaidBill()}, &stubBillReader{}, &stubStore{})
	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{}); !errors.Is(err, ErrPaymentAmountMismatch) {
		t.Fatalf("want ErrPaymentAmountMismatch, got %v", err)
	}
}

func TestApprove_RejectsWhenPaymentAlreadyExists(t *testing.T) {
	// Concurrent approval backstop: unique(bill_id) rejects a second payment.
	sub := &stubSubmissionRepo{forUpdate: pendingSubmission()}
	pay := &stubPaymentRepo{bill: unpaidBill(), assignment: "active", insertErr: repository.ErrDuplicatePayment}
	svc := newManualSvc(sub, pay, &stubBillReader{}, &stubStore{})
	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{}); !errors.Is(err, ErrDuplicatePayment) {
		t.Fatalf("want ErrDuplicatePayment, got %v", err)
	}
	if sub.approved {
		t.Fatal("submission must not be marked approved when payment insert fails")
	}
}

func TestApprove_RejectsPaidBill(t *testing.T) {
	bill := unpaidBill()
	bill.Status = "paid"
	svc := newManualSvc(&stubSubmissionRepo{forUpdate: pendingSubmission()}, &stubPaymentRepo{bill: bill}, &stubBillReader{}, &stubStore{})
	if _, err := svc.Approve(context.Background(), "owner-1", "sub-1", "reviewer-1", model.ReviewSubmissionInput{}); !errors.Is(err, ErrBillAlreadyPaid) {
		t.Fatalf("want ErrBillAlreadyPaid, got %v", err)
	}
}

// --- owner reject --------------------------------------------------------

func TestReject_Success(t *testing.T) {
	sub := &stubSubmissionRepo{detail: pendingSubmission()}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	err := svc.Reject(context.Background(), "owner-1", "sub-1", "reviewer-1", model.RejectSubmissionInput{Reason: "payment_not_found"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sub.rejected || sub.rejectReason != "payment_not_found" || sub.rejectedBy != "reviewer-1" {
		t.Fatalf("rejection not recorded: %+v", sub)
	}
}

func TestReject_RequiresReason(t *testing.T) {
	svc := newManualSvc(&stubSubmissionRepo{detail: pendingSubmission()}, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	if err := svc.Reject(context.Background(), "owner-1", "sub-1", "reviewer-1", model.RejectSubmissionInput{}); !errors.Is(err, ErrRejectReasonRequired) {
		t.Fatalf("want ErrRejectReasonRequired, got %v", err)
	}
}

func TestReject_RejectsNonPending(t *testing.T) {
	s := pendingSubmission()
	s.Status = "approved"
	svc := newManualSvc(&stubSubmissionRepo{detail: s}, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	if err := svc.Reject(context.Background(), "owner-1", "sub-1", "reviewer-1", model.RejectSubmissionInput{Reason: "other"}); !errors.Is(err, ErrSubmissionNotPending) {
		t.Fatalf("want ErrSubmissionNotPending, got %v", err)
	}
}

func TestReject_LostRaceReturnsNotPending(t *testing.T) {
	// Passed the pending check but the conditioned UPDATE matched 0 rows (raced).
	sub := &stubSubmissionRepo{detail: pendingSubmission(), rejectErr: repository.ErrNotFound}
	svc := newManualSvc(sub, &stubPaymentRepo{}, &stubBillReader{}, &stubStore{})
	if err := svc.Reject(context.Background(), "owner-1", "sub-1", "reviewer-1", model.RejectSubmissionInput{Reason: "other"}); !errors.Is(err, ErrSubmissionNotPending) {
		t.Fatalf("want ErrSubmissionNotPending, got %v", err)
	}
}

// Ensure the stubs satisfy the interfaces.
var (
	_ repository.ManualPaymentSubmissionRepository = (*stubSubmissionRepo)(nil)
	_ repository.TenantPortalRepository            = (*stubBillReader)(nil)
	_ storage.Store                                = (*stubStore)(nil)
)

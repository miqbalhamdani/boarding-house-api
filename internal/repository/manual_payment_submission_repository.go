package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// ErrDuplicatePendingSubmission maps the partial unique index
// uq_pending_manual_submission_per_bill: a bill may have at most one
// pending_review submission at a time (Module 10).
var ErrDuplicatePendingSubmission = errors.New("a pending payment-proof submission already exists for this bill")

// manualPaymentSubmissionColumns is the canonical column list/order for
// scanning a submission row. Prefixed with "mps." for use inside joins.
const manualPaymentSubmissionColumns = `
	mps.id, mps.owner_id, mps.bill_id, mps.tenant_id, mps.submitted_amount, mps.payment_method,
	mps.transfer_date, mps.sender_account_name, mps.reference_number, mps.proof_url,
	mps.status, mps.tenant_notes, mps.review_reason, mps.review_notes, mps.reviewed_by, mps.reviewed_at,
	mps.created_at, mps.updated_at`

// manualPaymentSubmissionJoinColumns are the denormalized bill/tenant/room
// fields joined onto each submission row for the owner list/detail views.
const manualPaymentSubmissionJoinColumns = `
	COALESCE(t.full_name, '') AS tenant_name,
	COALESCE(rm.room_number, '') AS room_number,
	COALESCE(b.amount, 0) AS bill_amount,
	COALESCE(b.billing_month, '') AS billing_month`

// manualPaymentSubmissionJoins attaches the bill, tenant and room a submission
// points at. LEFT JOIN so a soft-deleted related row never drops the submission.
const manualPaymentSubmissionJoins = `
	FROM manual_payment_submissions mps
	LEFT JOIN bills b ON b.id = mps.bill_id
	LEFT JOIN tenants t ON t.id = mps.tenant_id
	LEFT JOIN rooms rm ON rm.id = b.room_id`

// ManualPaymentSubmissionRepository defines persistence for payment-proof
// submissions (Module 10). Every method filters by owner_id (BR-001); tenant
// methods additionally filter by tenant_id (BR-002).
type ManualPaymentSubmissionRepository interface {
	// Create inserts a new pending_review submission. It returns
	// ErrDuplicatePendingSubmission when the partial unique index rejects a
	// second pending submission for the same bill.
	Create(ctx context.Context, s model.ManualPaymentSubmission) (*model.ManualPaymentSubmission, error)
	// SetProofURL stores the object key after the proof upload succeeds.
	SetProofURL(ctx context.Context, id, ownerID, proofURL string) error
	// DeleteByID hard-deletes a submission (compensating cleanup when the proof
	// upload fails after the row was created).
	DeleteByID(ctx context.Context, id, ownerID string) error

	// GetLatestForBill returns the tenant's most recent submission for a bill.
	GetLatestForBill(ctx context.Context, billID, tenantID, ownerID string) (*model.ManualPaymentSubmission, error)
	// Cancel flips a tenant's own pending_review submission to cancelled.
	Cancel(ctx context.Context, id, tenantID, ownerID string) error

	// List returns the owner's submissions with denormalized bill/tenant/room
	// fields, filtered/paginated.
	List(ctx context.Context, ownerID string, f model.ListManualPaymentSubmissionsFilter) (*model.ListManualPaymentSubmissionsResult, error)
	// GetByIDForOwner returns one submission (with joins) scoped to the owner.
	GetByIDForOwner(ctx context.Context, id, ownerID string) (*model.ManualPaymentSubmission, error)

	// SubmissionForUpdate locks the submission row within tx so the approval
	// status transition is race-safe.
	SubmissionForUpdate(ctx context.Context, tx pgx.Tx, id, ownerID string) (*model.ManualPaymentSubmission, error)
	// MarkApproved transitions a locked submission to approved and stamps the
	// reviewer, within the approval transaction.
	MarkApproved(ctx context.Context, tx pgx.Tx, id, ownerID, reviewerID string, reviewNotes string, reviewedAt time.Time) error
	// MarkRejected transitions a pending_review submission to rejected. No
	// cross-table effects, so it runs outside a transaction; it returns
	// ErrNotFound when no pending_review row matched (missing/not-owned/raced).
	MarkRejected(ctx context.Context, id, ownerID, reviewerID, reason, reviewNotes string, reviewedAt time.Time) error
}

type manualPaymentSubmissionRepository struct {
	pool *pgxpool.Pool
}

// NewManualPaymentSubmissionRepository returns a Postgres-backed repository.
func NewManualPaymentSubmissionRepository(pool *pgxpool.Pool) ManualPaymentSubmissionRepository {
	return &manualPaymentSubmissionRepository{pool: pool}
}

// scanSubmission scans the base submission columns (no join fields).
func scanSubmission(row pgx.Row) (*model.ManualPaymentSubmission, error) {
	var s model.ManualPaymentSubmission
	err := row.Scan(
		&s.ID, &s.OwnerID, &s.BillID, &s.TenantID, &s.SubmittedAmount, &s.PaymentMethod,
		&s.TransferDate, &s.SenderAccountName, &s.ReferenceNumber, &s.ProofURL,
		&s.Status, &s.TenantNotes, &s.ReviewReason, &s.ReviewNotes, &s.ReviewedBy, &s.ReviewedAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	return &s, err
}

func (r *manualPaymentSubmissionRepository) Create(ctx context.Context, s model.ManualPaymentSubmission) (*model.ManualPaymentSubmission, error) {
	const q = `
		INSERT INTO manual_payment_submissions (
			owner_id, bill_id, tenant_id, submitted_amount, payment_method,
			transfer_date, sender_account_name, reference_number, status, tenant_notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending_review', $9)
		RETURNING ` + manualPaymentSubmissionColumns

	out, err := scanSubmission(r.pool.QueryRow(ctx, q,
		s.OwnerID, s.BillID, s.TenantID, s.SubmittedAmount, s.PaymentMethod,
		s.TransferDate, s.SenderAccountName, s.ReferenceNumber, s.TenantNotes,
	))
	if isUniqueViolation(err) {
		return nil, ErrDuplicatePendingSubmission
	}
	if err != nil {
		return nil, fmt.Errorf("create manual payment submission: %w", err)
	}
	return out, nil
}

func (r *manualPaymentSubmissionRepository) SetProofURL(ctx context.Context, id, ownerID, proofURL string) error {
	const q = `
		UPDATE manual_payment_submissions
		SET proof_url = $3, updated_at = now()
		WHERE id = $1 AND owner_id = $2`

	tag, err := r.pool.Exec(ctx, q, id, ownerID, proofURL)
	if err != nil {
		return fmt.Errorf("set proof url: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *manualPaymentSubmissionRepository) DeleteByID(ctx context.Context, id, ownerID string) error {
	const q = `DELETE FROM manual_payment_submissions WHERE id = $1 AND owner_id = $2`
	if _, err := r.pool.Exec(ctx, q, id, ownerID); err != nil {
		return fmt.Errorf("delete manual payment submission: %w", err)
	}
	return nil
}

func (r *manualPaymentSubmissionRepository) GetLatestForBill(ctx context.Context, billID, tenantID, ownerID string) (*model.ManualPaymentSubmission, error) {
	const q = `
		SELECT ` + manualPaymentSubmissionColumns + `
		FROM manual_payment_submissions mps
		WHERE mps.bill_id = $1 AND mps.tenant_id = $2 AND mps.owner_id = $3
		ORDER BY mps.created_at DESC
		LIMIT 1`

	s, err := scanSubmission(r.pool.QueryRow(ctx, q, billID, tenantID, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest submission for bill: %w", err)
	}
	return s, nil
}

func (r *manualPaymentSubmissionRepository) Cancel(ctx context.Context, id, tenantID, ownerID string) error {
	// Conditioned on pending_review: a cancelled/approved/rejected submission
	// (or one owned by another tenant) matches nothing and yields ErrNotFound.
	const q = `
		UPDATE manual_payment_submissions
		SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND owner_id = $3 AND status = 'pending_review'`

	tag, err := r.pool.Exec(ctx, q, id, tenantID, ownerID)
	if err != nil {
		return fmt.Errorf("cancel submission: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *manualPaymentSubmissionRepository) List(ctx context.Context, ownerID string, f model.ListManualPaymentSubmissionsFilter) (*model.ListManualPaymentSubmissionsResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	offset := (f.Page - 1) * f.Limit

	const q = `
		SELECT ` + manualPaymentSubmissionColumns + `,
			` + manualPaymentSubmissionJoinColumns + `,
			COUNT(*) OVER() AS total_count
		` + manualPaymentSubmissionJoins + `
		WHERE mps.owner_id = $1
			AND ($2 = '' OR mps.status = $2)
			AND ($3 = '' OR mps.tenant_id::text = $3)
			AND ($4 = '' OR b.billing_month = $4)
		ORDER BY mps.created_at DESC
		LIMIT $5 OFFSET $6`

	rows, err := r.pool.Query(ctx, q, ownerID, f.Status, f.TenantID, f.BillingMonth, f.Limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list manual payment submissions: %w", err)
	}
	defer rows.Close()

	submissions := []*model.ManualPaymentSubmission{}
	var total int
	for rows.Next() {
		var s model.ManualPaymentSubmission
		if err := rows.Scan(
			&s.ID, &s.OwnerID, &s.BillID, &s.TenantID, &s.SubmittedAmount, &s.PaymentMethod,
			&s.TransferDate, &s.SenderAccountName, &s.ReferenceNumber, &s.ProofURL,
			&s.Status, &s.TenantNotes, &s.ReviewReason, &s.ReviewNotes, &s.ReviewedBy, &s.ReviewedAt,
			&s.CreatedAt, &s.UpdatedAt,
			&s.TenantName, &s.RoomNumber, &s.BillAmount, &s.BillingMonth,
			&total,
		); err != nil {
			return nil, fmt.Errorf("scan manual payment submission: %w", err)
		}
		submissions = append(submissions, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list manual payment submissions: %w", err)
	}

	return &model.ListManualPaymentSubmissionsResult{
		Submissions: submissions,
		Total:       total,
		Page:        f.Page,
		Limit:       f.Limit,
	}, nil
}

func (r *manualPaymentSubmissionRepository) GetByIDForOwner(ctx context.Context, id, ownerID string) (*model.ManualPaymentSubmission, error) {
	const q = `
		SELECT ` + manualPaymentSubmissionColumns + `,
			` + manualPaymentSubmissionJoinColumns + `
		` + manualPaymentSubmissionJoins + `
		WHERE mps.id = $1 AND mps.owner_id = $2`

	var s model.ManualPaymentSubmission
	err := r.pool.QueryRow(ctx, q, id, ownerID).Scan(
		&s.ID, &s.OwnerID, &s.BillID, &s.TenantID, &s.SubmittedAmount, &s.PaymentMethod,
		&s.TransferDate, &s.SenderAccountName, &s.ReferenceNumber, &s.ProofURL,
		&s.Status, &s.TenantNotes, &s.ReviewReason, &s.ReviewNotes, &s.ReviewedBy, &s.ReviewedAt,
		&s.CreatedAt, &s.UpdatedAt,
		&s.TenantName, &s.RoomNumber, &s.BillAmount, &s.BillingMonth,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get manual payment submission: %w", err)
	}
	return &s, nil
}

func (r *manualPaymentSubmissionRepository) SubmissionForUpdate(ctx context.Context, tx pgx.Tx, id, ownerID string) (*model.ManualPaymentSubmission, error) {
	const q = `
		SELECT ` + manualPaymentSubmissionColumns + `
		FROM manual_payment_submissions mps
		WHERE mps.id = $1 AND mps.owner_id = $2
		FOR UPDATE`

	s, err := scanSubmission(tx.QueryRow(ctx, q, id, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock submission: %w", err)
	}
	return s, nil
}

func (r *manualPaymentSubmissionRepository) MarkApproved(ctx context.Context, tx pgx.Tx, id, ownerID, reviewerID, reviewNotes string, reviewedAt time.Time) error {
	const q = `
		UPDATE manual_payment_submissions
		SET status = 'approved', reviewed_by = $3, reviewed_at = $4,
			review_notes = $5, updated_at = now()
		WHERE id = $1 AND owner_id = $2`
	tag, err := tx.Exec(ctx, q, id, ownerID, reviewerID, reviewedAt, nullableText(reviewNotes))
	if err != nil {
		return fmt.Errorf("mark submission approved: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *manualPaymentSubmissionRepository) MarkRejected(ctx context.Context, id, ownerID, reviewerID, reason, reviewNotes string, reviewedAt time.Time) error {
	const q = `
		UPDATE manual_payment_submissions
		SET status = 'rejected', review_reason = $3, review_notes = $4,
			reviewed_by = $5, reviewed_at = $6, updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND status = 'pending_review'`
	tag, err := r.pool.Exec(ctx, q, id, ownerID, reason, nullableText(reviewNotes), reviewerID, reviewedAt)
	if err != nil {
		return fmt.Errorf("mark submission rejected: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// nullableText returns nil for an empty string so optional text columns persist
// as NULL rather than "".
func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

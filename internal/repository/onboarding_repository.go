package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// Errors surfaced by the onboarding repository when a partial unique index
// rejects a second active/pending assignment (the race-safe backstop for the
// pre-insert checks the service performs).
var (
	ErrRoomAssignmentExists   = errors.New("room already has an active or pending assignment")
	ErrTenantAssignmentExists = errors.New("tenant already has an active or pending assignment")
	// ErrDuplicateBill maps the unique(room_assignment_id, billing_month)
	// violation (BR-016): a bill already exists for that assignment and month.
	ErrDuplicateBill = errors.New("bill already exists for this assignment and billing month")
)

// OnboardingRepository exposes the persistence primitives the onboarding
// service composes inside a single transaction. Every method filters by
// owner_id to enforce owner isolation.
type OnboardingRepository interface {
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// RoomStatusForUpdate locks the room row and returns its status.
	RoomStatusForUpdate(ctx context.Context, tx pgx.Tx, roomID, ownerID string) (string, error)
	// TenantStatusForUpdate locks the tenant row and returns its status.
	TenantStatusForUpdate(ctx context.Context, tx pgx.Tx, tenantID, ownerID string) (string, error)

	CountActiveAssignmentsForRoom(ctx context.Context, tx pgx.Tx, roomID, ownerID string) (int, error)
	CountActiveAssignmentsForTenant(ctx context.Context, tx pgx.Tx, tenantID, ownerID string) (int, error)

	CreateAssignment(ctx context.Context, tx pgx.Tx, a model.RoomAssignment) (*model.RoomAssignment, error)
	CreateBill(ctx context.Context, tx pgx.Tx, b model.Bill) (*model.Bill, error)

	UpdateRoomStatus(ctx context.Context, tx pgx.Tx, roomID, ownerID, status string) error
	UpdateTenantStatus(ctx context.Context, tx pgx.Tx, tenantID, ownerID, status string) error

	// AssignmentForUpdate locks and returns an assignment for the cancel flow.
	AssignmentForUpdate(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) (*model.RoomAssignment, error)
	UpdateAssignmentStatus(ctx context.Context, tx pgx.Tx, assignmentID, ownerID, status string) error
	// CancelUnpaidBillsForAssignment cancels still-collectible bills for an
	// assignment (used when an onboarding is cancelled before first payment).
	CancelUnpaidBillsForAssignment(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) error
}

type onboardingRepository struct {
	pool *pgxpool.Pool
}

// NewOnboardingRepository returns a Postgres-backed OnboardingRepository.
func NewOnboardingRepository(pool *pgxpool.Pool) OnboardingRepository {
	return &onboardingRepository{pool: pool}
}

func (r *onboardingRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

func (r *onboardingRepository) RoomStatusForUpdate(ctx context.Context, tx pgx.Tx, roomID, ownerID string) (string, error) {
	const q = `
		SELECT status FROM rooms
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		FOR UPDATE`

	var status string
	err := tx.QueryRow(ctx, q, roomID, ownerID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock room: %w", err)
	}
	return status, nil
}

func (r *onboardingRepository) TenantStatusForUpdate(ctx context.Context, tx pgx.Tx, tenantID, ownerID string) (string, error) {
	const q = `
		SELECT status FROM tenants
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		FOR UPDATE`

	var status string
	err := tx.QueryRow(ctx, q, tenantID, ownerID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock tenant: %w", err)
	}
	return status, nil
}

func (r *onboardingRepository) CountActiveAssignmentsForRoom(ctx context.Context, tx pgx.Tx, roomID, ownerID string) (int, error) {
	const q = `
		SELECT count(*) FROM room_assignments
		WHERE room_id = $1 AND owner_id = $2
			AND status IN ('pending_payment', 'active')
			AND deleted_at IS NULL`

	var n int
	if err := tx.QueryRow(ctx, q, roomID, ownerID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count room assignments: %w", err)
	}
	return n, nil
}

func (r *onboardingRepository) CountActiveAssignmentsForTenant(ctx context.Context, tx pgx.Tx, tenantID, ownerID string) (int, error) {
	const q = `
		SELECT count(*) FROM room_assignments
		WHERE tenant_id = $1 AND owner_id = $2
			AND status IN ('pending_payment', 'active')
			AND deleted_at IS NULL`

	var n int
	if err := tx.QueryRow(ctx, q, tenantID, ownerID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count tenant assignments: %w", err)
	}
	return n, nil
}

const assignmentCols = `
	id, owner_id, tenant_id, room_id, start_date, end_date,
	monthly_rent, payment_due_day, status, created_at, updated_at`

func scanAssignment(row pgx.Row) (*model.RoomAssignment, error) {
	var a model.RoomAssignment
	err := row.Scan(
		&a.ID, &a.OwnerID, &a.TenantID, &a.RoomID, &a.StartDate, &a.EndDate,
		&a.MonthlyRent, &a.PaymentDueDay, &a.Status, &a.CreatedAt, &a.UpdatedAt,
	)
	return &a, err
}

func (r *onboardingRepository) CreateAssignment(ctx context.Context, tx pgx.Tx, a model.RoomAssignment) (*model.RoomAssignment, error) {
	const q = `
		INSERT INTO room_assignments (
			owner_id, tenant_id, room_id, start_date, monthly_rent, payment_due_day, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + assignmentCols

	out, err := scanAssignment(tx.QueryRow(ctx, q,
		a.OwnerID, a.TenantID, a.RoomID, a.StartDate, a.MonthlyRent, a.PaymentDueDay, a.Status,
	))
	if e := mapAssignmentUnique(err); e != nil {
		return nil, e
	}
	if err != nil {
		return nil, fmt.Errorf("create room assignment: %w", err)
	}
	return out, nil
}

func (r *onboardingRepository) CreateBill(ctx context.Context, tx pgx.Tx, b model.Bill) (*model.Bill, error) {
	const q = `
		INSERT INTO bills (
			owner_id, tenant_id, room_id, room_assignment_id, bill_number, bill_type,
			billing_month, billing_period_start, billing_period_end, amount, due_date, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING
			id, owner_id, tenant_id, room_id, room_assignment_id, bill_number, bill_type,
			billing_month, billing_period_start, billing_period_end, amount, due_date,
			status, paid_at, created_at, updated_at`

	var out model.Bill
	err := tx.QueryRow(ctx, q,
		b.OwnerID, b.TenantID, b.RoomID, b.RoomAssignmentID, b.BillNumber, b.BillType,
		b.BillingMonth, b.BillingPeriodStart, b.BillingPeriodEnd, b.Amount, b.DueDate, b.Status,
	).Scan(
		&out.ID, &out.OwnerID, &out.TenantID, &out.RoomID, &out.RoomAssignmentID,
		&out.BillNumber, &out.BillType, &out.BillingMonth, &out.BillingPeriodStart,
		&out.BillingPeriodEnd, &out.Amount, &out.DueDate, &out.Status, &out.PaidAt,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if isUniqueViolation(err) {
		// (room_assignment_id, billing_month) duplicate — first bill already exists.
		return nil, ErrDuplicateBill
	}
	if err != nil {
		return nil, fmt.Errorf("create bill: %w", err)
	}
	return &out, nil
}

func (r *onboardingRepository) UpdateRoomStatus(ctx context.Context, tx pgx.Tx, roomID, ownerID, status string) error {
	const q = `
		UPDATE rooms SET status = $3, updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	tag, err := tx.Exec(ctx, q, roomID, ownerID, status)
	if err != nil {
		return fmt.Errorf("update room status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *onboardingRepository) UpdateTenantStatus(ctx context.Context, tx pgx.Tx, tenantID, ownerID, status string) error {
	const q = `
		UPDATE tenants SET status = $3, updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	tag, err := tx.Exec(ctx, q, tenantID, ownerID, status)
	if err != nil {
		return fmt.Errorf("update tenant status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *onboardingRepository) AssignmentForUpdate(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) (*model.RoomAssignment, error) {
	const q = `
		SELECT ` + assignmentCols + `
		FROM room_assignments
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		FOR UPDATE`

	out, err := scanAssignment(tx.QueryRow(ctx, q, assignmentID, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock room assignment: %w", err)
	}
	return out, nil
}

func (r *onboardingRepository) UpdateAssignmentStatus(ctx context.Context, tx pgx.Tx, assignmentID, ownerID, status string) error {
	const q = `
		UPDATE room_assignments SET status = $3, updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	tag, err := tx.Exec(ctx, q, assignmentID, ownerID, status)
	if err != nil {
		return fmt.Errorf("update assignment status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *onboardingRepository) CancelUnpaidBillsForAssignment(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) error {
	const q = `
		UPDATE bills SET status = 'cancelled', updated_at = now()
		WHERE room_assignment_id = $1 AND owner_id = $2
			AND status IN ('unpaid', 'overdue', 'gateway_pending')
			AND deleted_at IS NULL`

	if _, err := tx.Exec(ctx, q, assignmentID, ownerID); err != nil {
		return fmt.Errorf("cancel unpaid bills: %w", err)
	}
	return nil
}

// mapAssignmentUnique translates the partial unique index violations into the
// matching domain error so concurrent assignment attempts fail cleanly.
func mapAssignmentUnique(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil
	}
	switch pgErr.ConstraintName {
	case "uq_room_assignments_active_room":
		return ErrRoomAssignmentExists
	case "uq_room_assignments_active_tenant":
		return ErrTenantAssignmentExists
	default:
		return ErrRoomAssignmentExists
	}
}

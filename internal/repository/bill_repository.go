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

// billColumns is the canonical column list/order for scanning a bill row.
const billColumns = `
	id, owner_id, tenant_id, room_id, room_assignment_id, bill_number, bill_type,
	billing_month, billing_period_start, billing_period_end, amount, due_date,
	status, paid_at, created_at, updated_at`

// BillRepository defines persistence operations for monthly rent bills. Every
// method filters by owner_id to enforce owner isolation (BR-001).
type BillRepository interface {
	List(ctx context.Context, ownerID string, f model.ListBillsFilter) (*model.ListBillsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Bill, error)

	// ActiveAssignments returns the owner's active room assignments, the only
	// assignments eligible for automatic monthly billing (BR-015).
	ActiveAssignments(ctx context.Context, ownerID string) ([]*model.RoomAssignment, error)
	// BeginTx starts a transaction the service uses to generate a month's
	// bills atomically.
	BeginTx(ctx context.Context) (pgx.Tx, error)
	// InsertBillIfAbsent inserts a bill within tx, relying on the
	// unique(room_assignment_id, billing_month) constraint for idempotency
	// (BR-016). It returns false when a bill already exists for that month.
	InsertBillIfAbsent(ctx context.Context, tx pgx.Tx, b model.Bill) (bool, error)
	// MarkOverdue flips unpaid bills whose due_date has passed to overdue
	// (BR-018) and returns how many rows changed.
	MarkOverdue(ctx context.Context, ownerID string, today time.Time) (int, error)
}

type billRepository struct {
	pool *pgxpool.Pool
}

// NewBillRepository returns a Postgres-backed BillRepository.
func NewBillRepository(pool *pgxpool.Pool) BillRepository {
	return &billRepository{pool: pool}
}

func scanBill(row pgx.Row) (*model.Bill, error) {
	var b model.Bill
	err := row.Scan(
		&b.ID, &b.OwnerID, &b.TenantID, &b.RoomID, &b.RoomAssignmentID,
		&b.BillNumber, &b.BillType, &b.BillingMonth, &b.BillingPeriodStart,
		&b.BillingPeriodEnd, &b.Amount, &b.DueDate, &b.Status, &b.PaidAt,
		&b.CreatedAt, &b.UpdatedAt,
	)
	return &b, err
}

func (r *billRepository) List(ctx context.Context, ownerID string, f model.ListBillsFilter) (*model.ListBillsResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	offset := (f.Page - 1) * f.Limit

	const q = `
		SELECT ` + billColumns + `,
			COUNT(*) OVER() AS total_count
		FROM bills
		WHERE owner_id = $1
			AND deleted_at IS NULL
			AND ($2 = '' OR status = $2)
			AND ($3 = '' OR billing_month = $3)
			AND ($4 = '' OR tenant_id::text = $4)
			AND ($5 = '' OR room_id::text = $5)
		ORDER BY created_at DESC
		LIMIT $6 OFFSET $7`

	rows, err := r.pool.Query(ctx, q, ownerID, f.Status, f.BillingMonth, f.TenantID, f.RoomID, f.Limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list bills: %w", err)
	}
	defer rows.Close()

	bills := []*model.Bill{}
	var total int
	for rows.Next() {
		var b model.Bill
		if err := rows.Scan(
			&b.ID, &b.OwnerID, &b.TenantID, &b.RoomID, &b.RoomAssignmentID,
			&b.BillNumber, &b.BillType, &b.BillingMonth, &b.BillingPeriodStart,
			&b.BillingPeriodEnd, &b.Amount, &b.DueDate, &b.Status, &b.PaidAt,
			&b.CreatedAt, &b.UpdatedAt,
			&total,
		); err != nil {
			return nil, fmt.Errorf("scan bill: %w", err)
		}
		bills = append(bills, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bills: %w", err)
	}

	return &model.ListBillsResult{
		Bills: bills,
		Total: total,
		Page:  f.Page,
		Limit: f.Limit,
	}, nil
}

func (r *billRepository) GetByID(ctx context.Context, id, ownerID string) (*model.Bill, error) {
	const q = `
		SELECT ` + billColumns + `
		FROM bills
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	b, err := scanBill(r.pool.QueryRow(ctx, q, id, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get bill by id: %w", err)
	}
	return b, nil
}

func (r *billRepository) ActiveAssignments(ctx context.Context, ownerID string) ([]*model.RoomAssignment, error) {
	const q = `
		SELECT ` + assignmentCols + `
		FROM room_assignments
		WHERE owner_id = $1 AND status = 'active' AND deleted_at IS NULL
		ORDER BY created_at`

	rows, err := r.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list active assignments: %w", err)
	}
	defer rows.Close()

	assignments := []*model.RoomAssignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active assignment: %w", err)
		}
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active assignments: %w", err)
	}
	return assignments, nil
}

func (r *billRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

func (r *billRepository) InsertBillIfAbsent(ctx context.Context, tx pgx.Tx, b model.Bill) (bool, error) {
	const q = `
		INSERT INTO bills (
			owner_id, tenant_id, room_id, room_assignment_id, bill_number, bill_type,
			billing_month, billing_period_start, billing_period_end, amount, due_date, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (room_assignment_id, billing_month) DO NOTHING
		RETURNING id`

	var id string
	err := tx.QueryRow(ctx, q,
		b.OwnerID, b.TenantID, b.RoomID, b.RoomAssignmentID, b.BillNumber, b.BillType,
		b.BillingMonth, b.BillingPeriodStart, b.BillingPeriodEnd, b.Amount, b.DueDate, b.Status,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// A bill already exists for this assignment + billing month (BR-016).
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert bill: %w", err)
	}
	return true, nil
}

func (r *billRepository) MarkOverdue(ctx context.Context, ownerID string, today time.Time) (int, error) {
	const q = `
		UPDATE bills
		SET status = 'overdue', updated_at = now()
		WHERE owner_id = $1
			AND status = 'unpaid'
			AND due_date < $2
			AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, ownerID, today)
	if err != nil {
		return 0, fmt.Errorf("mark overdue bills: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

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

// ErrDuplicatePayment maps the unique(bill_id) violation on payments (BR-030):
// a successful payment already exists for the bill.
var ErrDuplicatePayment = errors.New("a payment already exists for this bill")

// paymentColumns is the canonical column list/order for scanning a payment row.
const paymentColumns = `
	id, owner_id, bill_id, tenant_id, room_id, amount, payment_date,
	payment_method, payment_source, gateway_transaction_id, reference_number,
	notes, created_at, updated_at`

// PaymentRepository defines persistence operations for full payments. Every
// method filters by owner_id to enforce owner isolation (BR-001).
type PaymentRepository interface {
	List(ctx context.Context, ownerID string, f model.ListPaymentsFilter) (*model.ListPaymentsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Payment, error)

	// BeginTx starts the transaction that wraps payment creation, the bill
	// status update, and first-payment activation as one atomic unit.
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// BillForUpdate locks the owner's bill row and returns it, so the payment
	// amount/status checks are race-safe against concurrent payment attempts.
	BillForUpdate(ctx context.Context, tx pgx.Tx, billID, ownerID string) (*model.Bill, error)
	// InsertPayment writes the successful payment. It returns ErrDuplicatePayment
	// when the unique(bill_id) constraint rejects a second payment (BR-030).
	InsertPayment(ctx context.Context, tx pgx.Tx, p model.Payment) (*model.Payment, error)
	// MarkBillPaid flips a bill to paid and stamps paid_at (BR-029).
	MarkBillPaid(ctx context.Context, tx pgx.Tx, billID, ownerID string, paidAt time.Time) error

	// AssignmentStatusForUpdate locks the assignment and returns its status so
	// the service can decide whether this is the activating first payment.
	AssignmentStatusForUpdate(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) (string, error)
	ActivateAssignment(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) error
	ActivateTenant(ctx context.Context, tx pgx.Tx, tenantID, ownerID string) error
	OccupyRoom(ctx context.Context, tx pgx.Tx, roomID, ownerID string) error
}

type paymentRepository struct {
	pool *pgxpool.Pool
}

// NewPaymentRepository returns a Postgres-backed PaymentRepository.
func NewPaymentRepository(pool *pgxpool.Pool) PaymentRepository {
	return &paymentRepository{pool: pool}
}

func scanPayment(row pgx.Row) (*model.Payment, error) {
	var p model.Payment
	err := row.Scan(
		&p.ID, &p.OwnerID, &p.BillID, &p.TenantID, &p.RoomID, &p.Amount,
		&p.PaymentDate, &p.PaymentMethod, &p.PaymentSource, &p.GatewayTransactionID,
		&p.ReferenceNumber, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
	)
	return &p, err
}

func (r *paymentRepository) List(ctx context.Context, ownerID string, f model.ListPaymentsFilter) (*model.ListPaymentsResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	offset := (f.Page - 1) * f.Limit

	const q = `
		SELECT ` + paymentColumns + `,
			COUNT(*) OVER() AS total_count
		FROM payments
		WHERE owner_id = $1
			AND ($2 = '' OR tenant_id::text = $2)
			AND ($3 = '' OR to_char(payment_date, 'YYYY-MM') = $3)
		ORDER BY payment_date DESC
		LIMIT $4 OFFSET $5`

	rows, err := r.pool.Query(ctx, q, ownerID, f.TenantID, f.Month, f.Limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	payments := []*model.Payment{}
	var total int
	for rows.Next() {
		var p model.Payment
		if err := rows.Scan(
			&p.ID, &p.OwnerID, &p.BillID, &p.TenantID, &p.RoomID, &p.Amount,
			&p.PaymentDate, &p.PaymentMethod, &p.PaymentSource, &p.GatewayTransactionID,
			&p.ReferenceNumber, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
			&total,
		); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		payments = append(payments, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}

	return &model.ListPaymentsResult{
		Payments: payments,
		Total:    total,
		Page:     f.Page,
		Limit:    f.Limit,
	}, nil
}

func (r *paymentRepository) GetByID(ctx context.Context, id, ownerID string) (*model.Payment, error) {
	const q = `
		SELECT ` + paymentColumns + `
		FROM payments
		WHERE id = $1 AND owner_id = $2`

	p, err := scanPayment(r.pool.QueryRow(ctx, q, id, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get payment by id: %w", err)
	}
	return p, nil
}

func (r *paymentRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

func (r *paymentRepository) BillForUpdate(ctx context.Context, tx pgx.Tx, billID, ownerID string) (*model.Bill, error) {
	const q = `
		SELECT
			id, owner_id, tenant_id, room_id, room_assignment_id, bill_number, bill_type,
			billing_month, billing_period_start, billing_period_end, amount, due_date,
			status, paid_at, created_at, updated_at
		FROM bills
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		FOR UPDATE`

	var b model.Bill
	err := tx.QueryRow(ctx, q, billID, ownerID).Scan(
		&b.ID, &b.OwnerID, &b.TenantID, &b.RoomID, &b.RoomAssignmentID,
		&b.BillNumber, &b.BillType, &b.BillingMonth, &b.BillingPeriodStart,
		&b.BillingPeriodEnd, &b.Amount, &b.DueDate, &b.Status, &b.PaidAt,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock bill: %w", err)
	}
	return &b, nil
}

func (r *paymentRepository) InsertPayment(ctx context.Context, tx pgx.Tx, p model.Payment) (*model.Payment, error) {
	const q = `
		INSERT INTO payments (
			owner_id, bill_id, tenant_id, room_id, amount, payment_date,
			payment_method, payment_source, gateway_transaction_id, reference_number, notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING ` + paymentColumns

	out, err := scanPayment(tx.QueryRow(ctx, q,
		p.OwnerID, p.BillID, p.TenantID, p.RoomID, p.Amount, p.PaymentDate,
		p.PaymentMethod, p.PaymentSource, p.GatewayTransactionID, p.ReferenceNumber, p.Notes,
	))
	if isUniqueViolation(err) {
		// unique(bill_id): a successful payment already exists for the bill.
		return nil, ErrDuplicatePayment
	}
	if err != nil {
		return nil, fmt.Errorf("insert payment: %w", err)
	}
	return out, nil
}

func (r *paymentRepository) MarkBillPaid(ctx context.Context, tx pgx.Tx, billID, ownerID string, paidAt time.Time) error {
	const q = `
		UPDATE bills
		SET status = 'paid', paid_at = $3, updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	tag, err := tx.Exec(ctx, q, billID, ownerID, paidAt)
	if err != nil {
		return fmt.Errorf("mark bill paid: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *paymentRepository) AssignmentStatusForUpdate(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) (string, error) {
	const q = `
		SELECT status FROM room_assignments
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		FOR UPDATE`

	var status string
	err := tx.QueryRow(ctx, q, assignmentID, ownerID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock assignment: %w", err)
	}
	return status, nil
}

func (r *paymentRepository) ActivateAssignment(ctx context.Context, tx pgx.Tx, assignmentID, ownerID string) error {
	const q = `
		UPDATE room_assignments SET status = 'active', updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`
	return execExpectOne(ctx, tx, q, "activate assignment", assignmentID, ownerID)
}

func (r *paymentRepository) ActivateTenant(ctx context.Context, tx pgx.Tx, tenantID, ownerID string) error {
	const q = `
		UPDATE tenants SET status = 'active', updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`
	return execExpectOne(ctx, tx, q, "activate tenant", tenantID, ownerID)
}

func (r *paymentRepository) OccupyRoom(ctx context.Context, tx pgx.Tx, roomID, ownerID string) error {
	const q = `
		UPDATE rooms SET status = 'occupied', updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`
	return execExpectOne(ctx, tx, q, "occupy room", roomID, ownerID)
}

// execExpectOne runs an owner-scoped update that must touch exactly one row,
// returning ErrNotFound when nothing matched (wrong owner or soft-deleted).
func execExpectOne(ctx context.Context, tx pgx.Tx, q, op string, id, ownerID string) error {
	tag, err := tx.Exec(ctx, q, id, ownerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

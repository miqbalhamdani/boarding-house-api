package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// TenantPortalRepository defines the read queries backing the tenant portal.
// Every method filters by BOTH tenant_id and owner_id derived from the tenant
// token, so a tenant can only ever read their own data (BR-002).
type TenantPortalRepository interface {
	MyRoom(ctx context.Context, tenantID, ownerID string) (*model.TenantRoomView, error)
	ListBills(ctx context.Context, tenantID, ownerID string, f model.TenantListBillsFilter) (*model.ListBillsResult, error)
	GetBill(ctx context.Context, billID, tenantID, ownerID string) (*model.Bill, error)
	ListPayments(ctx context.Context, tenantID, ownerID string, f model.TenantListPaymentsFilter) (*model.ListPaymentsResult, error)
}

type tenantPortalRepository struct {
	pool *pgxpool.Pool
}

// NewTenantPortalRepository returns a Postgres-backed TenantPortalRepository.
func NewTenantPortalRepository(pool *pgxpool.Pool) TenantPortalRepository {
	return &tenantPortalRepository{pool: pool}
}

func (r *tenantPortalRepository) MyRoom(ctx context.Context, tenantID, ownerID string) (*model.TenantRoomView, error) {
	// The tenant's current room is their active or pending assignment; ended and
	// cancelled assignments are historical and not "my room". Newest wins.
	const q = `
		SELECT
			ra.id, ra.status, ra.start_date, ra.end_date, ra.monthly_rent, ra.payment_due_day,
			r.id, r.room_number, r.room_name, r.status, r.notes
		FROM room_assignments ra
		JOIN rooms r ON r.id = ra.room_id
		WHERE ra.tenant_id = $1 AND ra.owner_id = $2
			AND ra.status IN ('pending_payment', 'active')
			AND ra.deleted_at IS NULL
		ORDER BY ra.created_at DESC
		LIMIT 1`

	var v model.TenantRoomView
	err := r.pool.QueryRow(ctx, q, tenantID, ownerID).Scan(
		&v.RoomAssignmentID, &v.AssignmentStatus, &v.StartDate, &v.EndDate,
		&v.MonthlyRent, &v.PaymentDueDay,
		&v.RoomID, &v.RoomNumber, &v.RoomName, &v.RoomStatus, &v.Notes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get my room: %w", err)
	}
	return &v, nil
}

func (r *tenantPortalRepository) ListBills(ctx context.Context, tenantID, ownerID string, f model.TenantListBillsFilter) (*model.ListBillsResult, error) {
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
		WHERE tenant_id = $1 AND owner_id = $2
			AND deleted_at IS NULL
			AND ($3 = '' OR status = $3)
		ORDER BY billing_month DESC, created_at DESC
		LIMIT $4 OFFSET $5`

	rows, err := r.pool.Query(ctx, q, tenantID, ownerID, f.Status, f.Limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tenant bills: %w", err)
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
			return nil, fmt.Errorf("scan tenant bill: %w", err)
		}
		bills = append(bills, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tenant bills: %w", err)
	}

	return &model.ListBillsResult{
		Bills: bills,
		Total: total,
		Page:  f.Page,
		Limit: f.Limit,
	}, nil
}

func (r *tenantPortalRepository) GetBill(ctx context.Context, billID, tenantID, ownerID string) (*model.Bill, error) {
	const q = `
		SELECT ` + billColumns + `
		FROM bills
		WHERE id = $1 AND tenant_id = $2 AND owner_id = $3 AND deleted_at IS NULL`

	b, err := scanBill(r.pool.QueryRow(ctx, q, billID, tenantID, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant bill by id: %w", err)
	}
	return b, nil
}

func (r *tenantPortalRepository) ListPayments(ctx context.Context, tenantID, ownerID string, f model.TenantListPaymentsFilter) (*model.ListPaymentsResult, error) {
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
		WHERE tenant_id = $1 AND owner_id = $2
		ORDER BY payment_date DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.pool.Query(ctx, q, tenantID, ownerID, f.Limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tenant payments: %w", err)
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
			return nil, fmt.Errorf("scan tenant payment: %w", err)
		}
		payments = append(payments, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tenant payments: %w", err)
	}

	return &model.ListPaymentsResult{
		Payments: payments,
		Total:    total,
		Page:     f.Page,
		Limit:    f.Limit,
	}, nil
}

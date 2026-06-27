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

// gatewayTxColumns is the canonical column list/order for scanning a gateway
// transaction row (raw_create_response is write-only and never scanned back).
const gatewayTxColumns = `
	id, owner_id, bill_id, tenant_id, provider, external_transaction_id,
	external_order_id, checkout_url, amount, currency, status, expires_at,
	paid_at, created_at, updated_at`

// GatewayRepository persists payment gateway checkout attempts. Module 08 only
// creates pending transactions for the tenant Pay Now flow; the webhook-driven
// transitions belong to the payment-gateway module. Every method is owner-scoped.
type GatewayRepository interface {
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// BillForUpdate locks the bill that belongs to BOTH this tenant and owner,
	// so the status check and the gateway_pending transition are race-safe.
	BillForUpdate(ctx context.Context, tx pgx.Tx, billID, tenantID, ownerID string) (*model.Bill, error)
	// ActivePendingTransaction returns the bill's still-valid pending checkout
	// (status pending and not expired), or ErrNotFound when none exists. This
	// enforces "one active pending transaction per bill" (BR-021) by letting the
	// service reuse the live checkout instead of creating a duplicate.
	ActivePendingTransaction(ctx context.Context, tx pgx.Tx, billID, ownerID string, now time.Time) (*model.GatewayTransaction, error)
	// InsertTransaction writes a new gateway transaction and stores the raw
	// provider response for audit (BR-034).
	InsertTransaction(ctx context.Context, tx pgx.Tx, gt model.GatewayTransaction, rawResponse []byte) (*model.GatewayTransaction, error)
	// SetBillGatewayPending flips the bill to gateway_pending (BR-023).
	SetBillGatewayPending(ctx context.Context, tx pgx.Tx, billID, ownerID string) error
}

type gatewayRepository struct {
	pool *pgxpool.Pool
}

// NewGatewayRepository returns a Postgres-backed GatewayRepository.
func NewGatewayRepository(pool *pgxpool.Pool) GatewayRepository {
	return &gatewayRepository{pool: pool}
}

func (r *gatewayRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

func scanGatewayTx(row pgx.Row) (*model.GatewayTransaction, error) {
	var gt model.GatewayTransaction
	err := row.Scan(
		&gt.ID, &gt.OwnerID, &gt.BillID, &gt.TenantID, &gt.Provider,
		&gt.ExternalTransactionID, &gt.ExternalOrderID, &gt.CheckoutURL,
		&gt.Amount, &gt.Currency, &gt.Status, &gt.ExpiresAt, &gt.PaidAt,
		&gt.CreatedAt, &gt.UpdatedAt,
	)
	return &gt, err
}

func (r *gatewayRepository) BillForUpdate(ctx context.Context, tx pgx.Tx, billID, tenantID, ownerID string) (*model.Bill, error) {
	const q = `
		SELECT
			id, owner_id, tenant_id, room_id, room_assignment_id, bill_number, bill_type,
			billing_month, billing_period_start, billing_period_end, amount, due_date,
			status, paid_at, created_at, updated_at
		FROM bills
		WHERE id = $1 AND tenant_id = $2 AND owner_id = $3 AND deleted_at IS NULL
		FOR UPDATE`

	var b model.Bill
	err := tx.QueryRow(ctx, q, billID, tenantID, ownerID).Scan(
		&b.ID, &b.OwnerID, &b.TenantID, &b.RoomID, &b.RoomAssignmentID,
		&b.BillNumber, &b.BillType, &b.BillingMonth, &b.BillingPeriodStart,
		&b.BillingPeriodEnd, &b.Amount, &b.DueDate, &b.Status, &b.PaidAt,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock tenant bill: %w", err)
	}
	return &b, nil
}

func (r *gatewayRepository) ActivePendingTransaction(ctx context.Context, tx pgx.Tx, billID, ownerID string, now time.Time) (*model.GatewayTransaction, error) {
	const q = `
		SELECT ` + gatewayTxColumns + `
		FROM payment_gateway_transactions
		WHERE bill_id = $1 AND owner_id = $2
			AND status = 'pending'
			AND (expires_at IS NULL OR expires_at > $3)
		ORDER BY created_at DESC
		LIMIT 1`

	gt, err := scanGatewayTx(tx.QueryRow(ctx, q, billID, ownerID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find active pending transaction: %w", err)
	}
	return gt, nil
}

func (r *gatewayRepository) InsertTransaction(ctx context.Context, tx pgx.Tx, gt model.GatewayTransaction, rawResponse []byte) (*model.GatewayTransaction, error) {
	const q = `
		INSERT INTO payment_gateway_transactions (
			owner_id, bill_id, tenant_id, provider, external_transaction_id,
			external_order_id, checkout_url, amount, currency, status, expires_at,
			raw_create_response
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb)
		RETURNING ` + gatewayTxColumns

	out, err := scanGatewayTx(tx.QueryRow(ctx, q,
		gt.OwnerID, gt.BillID, gt.TenantID, gt.Provider, gt.ExternalTransactionID,
		gt.ExternalOrderID, gt.CheckoutURL, gt.Amount, gt.Currency, gt.Status, gt.ExpiresAt,
		string(rawResponse),
	))
	if isUniqueViolation(err) {
		// unique(provider, external_order_id): the same order was already created.
		return nil, ErrDuplicateGatewayTransaction
	}
	if err != nil {
		return nil, fmt.Errorf("insert gateway transaction: %w", err)
	}
	return out, nil
}

func (r *gatewayRepository) SetBillGatewayPending(ctx context.Context, tx pgx.Tx, billID, ownerID string) error {
	const q = `
		UPDATE bills
		SET status = 'gateway_pending', updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	tag, err := tx.Exec(ctx, q, billID, ownerID)
	if err != nil {
		return fmt.Errorf("set bill gateway_pending: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrDuplicateGatewayTransaction maps the unique(provider, external_order_id)
// violation: a transaction already exists for this external order.
var ErrDuplicateGatewayTransaction = errors.New("a gateway transaction already exists for this order")

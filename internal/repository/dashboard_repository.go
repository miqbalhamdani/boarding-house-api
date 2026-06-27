package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// DashboardRepository aggregates owner-scoped overview metrics. Every subquery
// filters by owner_id to enforce owner isolation (BR-001, BR-031).
type DashboardRepository interface {
	// Summary computes all dashboard metrics for the owner. monthStart and
	// monthEnd bound the requested calendar month (half-open: [monthStart,
	// monthEnd)) used for the paid/collected metrics.
	Summary(ctx context.Context, ownerID string, monthStart, monthEnd time.Time) (*model.DashboardSummary, error)
}

type dashboardRepository struct {
	pool *pgxpool.Pool
}

// NewDashboardRepository returns a Postgres-backed DashboardRepository.
func NewDashboardRepository(pool *pgxpool.Pool) DashboardRepository {
	return &dashboardRepository{pool: pool}
}

// Summary runs a single round-trip of owner-scoped scalar subqueries.
//
// Current-state counts (rooms, tenants, outstanding bills) are not month-bound.
// paid_bills_this_month and collected_amount_this_month are sourced from the
// payments table — the canonical record of successful full payments — bounded
// by payment_date within the requested month. Gateway-pending bills have no
// payment row, so they are naturally excluded from collected amounts.
func (r *dashboardRepository) Summary(ctx context.Context, ownerID string, monthStart, monthEnd time.Time) (*model.DashboardSummary, error) {
	const q = `
		SELECT
			(SELECT COUNT(*) FROM rooms
				WHERE owner_id = $1 AND deleted_at IS NULL) AS total_rooms,
			(SELECT COUNT(*) FROM rooms
				WHERE owner_id = $1 AND deleted_at IS NULL AND status = 'available') AS available_rooms,
			(SELECT COUNT(*) FROM rooms
				WHERE owner_id = $1 AND deleted_at IS NULL AND status = 'occupied') AS occupied_rooms,
			(SELECT COUNT(*) FROM tenants
				WHERE owner_id = $1 AND deleted_at IS NULL AND status = 'active') AS active_tenants,
			(SELECT COUNT(*) FROM bills
				WHERE owner_id = $1 AND deleted_at IS NULL AND status = 'unpaid') AS unpaid_bills,
			(SELECT COUNT(*) FROM bills
				WHERE owner_id = $1 AND deleted_at IS NULL AND status = 'overdue') AS overdue_bills,
			(SELECT COUNT(*) FROM bills
				WHERE owner_id = $1 AND deleted_at IS NULL AND status = 'gateway_pending') AS gateway_pending_bills,
			(SELECT COUNT(*) FROM payments
				WHERE owner_id = $1 AND payment_date >= $2 AND payment_date < $3) AS paid_bills_this_month,
			(SELECT COALESCE(SUM(amount), 0) FROM payments
				WHERE owner_id = $1 AND payment_date >= $2 AND payment_date < $3) AS collected_amount_this_month`

	var s model.DashboardSummary
	err := r.pool.QueryRow(ctx, q, ownerID, monthStart, monthEnd).Scan(
		&s.TotalRooms,
		&s.AvailableRooms,
		&s.OccupiedRooms,
		&s.ActiveTenants,
		&s.UnpaidBills,
		&s.OverdueBills,
		&s.GatewayPendingBills,
		&s.PaidBillsThisMonth,
		&s.CollectedAmountThisMonth,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard summary: %w", err)
	}
	return &s, nil
}

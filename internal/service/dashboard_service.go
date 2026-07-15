package service

import (
	"context"
	"errors"
	"time"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// ErrInvalidMonth is returned when the dashboard month param is not a valid YYYY-MM.
var ErrInvalidMonth = errors.New("month must be a valid month (YYYY-MM)")

// dashboardListPreview caps each embedded dashboard list to a short preview.
// The list Total still reports the full owner-scoped count.
const dashboardListPreview = 5

// DashboardService implements the owner dashboard use case (Module 07).
type DashboardService interface {
	// Summary returns owner-scoped overview metrics for the given month. An
	// empty month defaults to the current calendar month (UTC).
	Summary(ctx context.Context, ownerID, month string) (*model.DashboardSummary, error)

	// Overview returns the summary counts plus short owner-scoped preview lists
	// (outstanding bills by status and recent payments for the month). An empty
	// month defaults to the current calendar month (UTC).
	Overview(ctx context.Context, ownerID, month string) (*model.DashboardView, error)
}

type dashboardService struct {
	repo        repository.DashboardRepository
	billRepo    repository.BillRepository
	paymentRepo repository.PaymentRepository
	now         func() time.Time
}

// NewDashboardService wires a DashboardService to its repositories. billRepo and
// paymentRepo back the embedded preview lists; every list query they run is
// owner-scoped, preserving owner isolation (BR-001, BR-031).
func NewDashboardService(repo repository.DashboardRepository, billRepo repository.BillRepository, paymentRepo repository.PaymentRepository) DashboardService {
	return &dashboardService{repo: repo, billRepo: billRepo, paymentRepo: paymentRepo, now: time.Now}
}

func (s *dashboardService) Summary(ctx context.Context, ownerID, month string) (*model.DashboardSummary, error) {
	month = s.resolveMonth(month)
	monthStart, err := time.ParseInLocation(monthLayout, month, time.UTC)
	if err != nil {
		return nil, ErrInvalidMonth
	}
	// Half-open month window [monthStart, monthEnd) for payment_date filtering.
	monthEnd := monthStart.AddDate(0, 1, 0)

	return s.repo.Summary(ctx, ownerID, monthStart, monthEnd)
}

func (s *dashboardService) Overview(ctx context.Context, ownerID, month string) (*model.DashboardView, error) {
	month = s.resolveMonth(month)

	summary, err := s.Summary(ctx, ownerID, month)
	if err != nil {
		return nil, err
	}

	// Outstanding-bill previews are current-state (status filter only), matching
	// the unpaid/overdue/gateway_pending counts.
	unpaid, err := s.billRepo.List(ctx, ownerID, model.ListBillsFilter{Status: "unpaid", Limit: dashboardListPreview})
	if err != nil {
		return nil, err
	}
	overdue, err := s.billRepo.List(ctx, ownerID, model.ListBillsFilter{Status: "overdue", Limit: dashboardListPreview})
	if err != nil {
		return nil, err
	}
	gatewayPending, err := s.billRepo.List(ctx, ownerID, model.ListBillsFilter{Status: "gateway_pending", Limit: dashboardListPreview})
	if err != nil {
		return nil, err
	}

	// Recent payments are scoped to the requested month, matching the collected
	// metrics.
	recentPayments, err := s.paymentRepo.List(ctx, ownerID, model.ListPaymentsFilter{Month: month, Limit: dashboardListPreview})
	if err != nil {
		return nil, err
	}

	return &model.DashboardView{
		DashboardSummary:    summary,
		UnpaidBills:         unpaid,
		OverdueBills:        overdue,
		GatewayPendingBills: gatewayPending,
		RecentPayments:      recentPayments,
	}, nil
}

// resolveMonth returns month unchanged, or the current calendar month (UTC) when
// month is empty. Invalid month strings are left as-is so the parse step in
// Summary rejects them with ErrInvalidMonth.
func (s *dashboardService) resolveMonth(month string) string {
	if month == "" {
		return s.now().UTC().Format(monthLayout)
	}
	return month
}

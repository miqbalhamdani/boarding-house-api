package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// stubDashboardRepo records the arguments Summary was called with so tests can
// assert owner isolation and the month window the service derives.
type stubDashboardRepo struct {
	gotOwnerID    string
	gotMonthStart time.Time
	gotMonthEnd   time.Time

	result *model.DashboardSummary
	err    error
}

func (s *stubDashboardRepo) Summary(_ context.Context, ownerID string, monthStart, monthEnd time.Time) (*model.DashboardSummary, error) {
	s.gotOwnerID = ownerID
	s.gotMonthStart = monthStart
	s.gotMonthEnd = monthEnd
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &model.DashboardSummary{}, nil
}

func TestDashboardSummary_DerivesOwnerFromArgument(t *testing.T) {
	// Owner isolation (BR-001, BR-031): the service must forward the
	// authenticated owner_id to the repository unchanged.
	repo := &stubDashboardRepo{}
	svc := NewDashboardService(repo, &stubBillRepo{}, &stubPaymentRepo{})

	if _, err := svc.Summary(context.Background(), "owner-42", "2026-07"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotOwnerID != "owner-42" {
		t.Fatalf("Summary used wrong owner_id: %q", repo.gotOwnerID)
	}
}

func TestDashboardSummary_ComputesHalfOpenMonthWindow(t *testing.T) {
	repo := &stubDashboardRepo{}
	svc := NewDashboardService(repo, &stubBillRepo{}, &stubPaymentRepo{})

	if _, err := svc.Summary(context.Background(), "owner-1", "2026-07"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if !repo.gotMonthStart.Equal(wantStart) {
		t.Fatalf("month start: want %v got %v", wantStart, repo.gotMonthStart)
	}
	if !repo.gotMonthEnd.Equal(wantEnd) {
		t.Fatalf("month end: want %v got %v", wantEnd, repo.gotMonthEnd)
	}
}

func TestDashboardSummary_DefaultsToCurrentMonth(t *testing.T) {
	repo := &stubDashboardRepo{}
	svc := &dashboardService{repo: repo, now: func() time.Time {
		return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	}}

	if _, err := svc.Summary(context.Background(), "owner-1", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if !repo.gotMonthStart.Equal(wantStart) || !repo.gotMonthEnd.Equal(wantEnd) {
		t.Fatalf("default month window wrong: start=%v end=%v", repo.gotMonthStart, repo.gotMonthEnd)
	}
}

func TestDashboardSummary_DecemberRollover(t *testing.T) {
	// AddDate must roll the year over for the upper bound.
	repo := &stubDashboardRepo{}
	svc := NewDashboardService(repo, &stubBillRepo{}, &stubPaymentRepo{})

	if _, err := svc.Summary(context.Background(), "owner-1", "2026-12"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantEnd := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if !repo.gotMonthEnd.Equal(wantEnd) {
		t.Fatalf("december rollover: want end %v got %v", wantEnd, repo.gotMonthEnd)
	}
}

func TestDashboardSummary_InvalidMonthRejected(t *testing.T) {
	repo := &stubDashboardRepo{}
	svc := NewDashboardService(repo, &stubBillRepo{}, &stubPaymentRepo{})

	for _, bad := range []string{"2026/07", "2026-13", "july", "26-07"} {
		if _, err := svc.Summary(context.Background(), "owner-1", bad); !errors.Is(err, ErrInvalidMonth) {
			t.Fatalf("month %q: want ErrInvalidMonth, got %v", bad, err)
		}
	}
	// Repo must not be touched when validation fails.
	if repo.gotOwnerID != "" {
		t.Fatalf("repo should not be called on invalid month")
	}
}

func TestDashboardSummary_ReturnsRepoResult(t *testing.T) {
	want := &model.DashboardSummary{
		TotalRooms:               10,
		AvailableRooms:           3,
		OccupiedRooms:            7,
		ActiveTenants:            7,
		UnpaidBills:              2,
		OverdueBills:             1,
		GatewayPendingBills:      1,
		PaidBillsThisMonth:       5,
		CollectedAmountThisMonth: 10000000,
	}
	svc := NewDashboardService(&stubDashboardRepo{result: want}, &stubBillRepo{}, &stubPaymentRepo{})

	got, err := svc.Summary(context.Background(), "owner-1", "2026-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *got != *want {
		t.Fatalf("summary mismatch: want %+v got %+v", want, got)
	}
}

func TestDashboardOverview_IncludesCountsAndLists(t *testing.T) {
	repo := &stubDashboardRepo{result: &model.DashboardSummary{TotalRooms: 10, UnpaidBills: 2}}
	billRepo := &stubBillRepo{listResult: &model.ListBillsResult{
		Bills: []*model.Bill{{ID: "bill-1"}}, Total: 1, Page: 1, Limit: dashboardListPreview,
	}}
	paymentRepo := &stubPaymentRepo{listResult: &model.ListPaymentsResult{
		Payments: []*model.Payment{{ID: "pay-1"}}, Total: 1, Page: 1, Limit: dashboardListPreview,
	}}
	svc := NewDashboardService(repo, billRepo, paymentRepo)

	view, err := svc.Overview(context.Background(), "owner-1", "2026-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.DashboardSummary == nil || view.TotalRooms != 10 {
		t.Fatalf("counts not embedded: %+v", view.DashboardSummary)
	}
	for name, list := range map[string]*model.ListBillsResult{
		"unpaid_bills_list":          view.UnpaidBills,
		"overdue_bills_list":         view.OverdueBills,
		"gateway_pending_bills_list": view.GatewayPendingBills,
	} {
		if list == nil || len(list.Bills) != 1 {
			t.Fatalf("%s not populated: %+v", name, list)
		}
	}
	if view.RecentPayments == nil || len(view.RecentPayments.Payments) != 1 {
		t.Fatalf("recent_payments not populated: %+v", view.RecentPayments)
	}
}

func TestDashboardOverview_EmptyListsWhenNone(t *testing.T) {
	billRepo := &stubBillRepo{listResult: &model.ListBillsResult{Bills: []*model.Bill{}, Limit: dashboardListPreview}}
	paymentRepo := &stubPaymentRepo{listResult: &model.ListPaymentsResult{Payments: []*model.Payment{}, Limit: dashboardListPreview}}
	svc := NewDashboardService(&stubDashboardRepo{}, billRepo, paymentRepo)

	view, err := svc.Overview(context.Background(), "owner-1", "2026-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.UnpaidBills == nil || view.OverdueBills == nil ||
		view.GatewayPendingBills == nil || view.RecentPayments == nil {
		t.Fatalf("lists must be non-nil even when empty: %+v", view)
	}
	if len(view.UnpaidBills.Bills) != 0 || len(view.RecentPayments.Payments) != 0 {
		t.Fatalf("lists should be empty: %+v", view)
	}
}

func TestDashboardOverview_ForwardsOwnerID(t *testing.T) {
	// Owner isolation (BR-001, BR-031): the authenticated owner_id must reach the
	// summary repo, the bill repo, and the payment repo unchanged.
	repo := &stubDashboardRepo{}
	billRepo := &stubBillRepo{listResult: &model.ListBillsResult{}}
	paymentRepo := &stubPaymentRepo{listResult: &model.ListPaymentsResult{}}
	svc := NewDashboardService(repo, billRepo, paymentRepo)

	if _, err := svc.Overview(context.Background(), "owner-42", "2026-07"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotOwnerID != "owner-42" {
		t.Fatalf("summary repo used wrong owner_id: %q", repo.gotOwnerID)
	}
	if billRepo.listOwner != "owner-42" {
		t.Fatalf("bill repo used wrong owner_id: %q", billRepo.listOwner)
	}
	if paymentRepo.listOwner != "owner-42" {
		t.Fatalf("payment repo used wrong owner_id: %q", paymentRepo.listOwner)
	}
}

func TestDashboardOverview_InvalidMonthRejected(t *testing.T) {
	// Validation failure must surface before any repo list query runs.
	billRepo := &stubBillRepo{}
	paymentRepo := &stubPaymentRepo{}
	svc := NewDashboardService(&stubDashboardRepo{}, billRepo, paymentRepo)

	if _, err := svc.Overview(context.Background(), "owner-1", "2026-13"); !errors.Is(err, ErrInvalidMonth) {
		t.Fatalf("want ErrInvalidMonth, got %v", err)
	}
	if billRepo.listOwner != "" || paymentRepo.listOwner != "" {
		t.Fatalf("list repos must not be called on invalid month")
	}
}

// Ensure the stub satisfies the repository interface.
var _ repository.DashboardRepository = (*stubDashboardRepo)(nil)

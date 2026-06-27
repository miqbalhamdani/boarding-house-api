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
	svc := NewDashboardService(repo)

	if _, err := svc.Summary(context.Background(), "owner-42", "2026-07"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotOwnerID != "owner-42" {
		t.Fatalf("Summary used wrong owner_id: %q", repo.gotOwnerID)
	}
}

func TestDashboardSummary_ComputesHalfOpenMonthWindow(t *testing.T) {
	repo := &stubDashboardRepo{}
	svc := NewDashboardService(repo)

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
	svc := NewDashboardService(repo)

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
	svc := NewDashboardService(repo)

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
	svc := NewDashboardService(&stubDashboardRepo{result: want})

	got, err := svc.Summary(context.Background(), "owner-1", "2026-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *got != *want {
		t.Fatalf("summary mismatch: want %+v got %+v", want, got)
	}
}

// Ensure the stub satisfies the repository interface.
var _ repository.DashboardRepository = (*stubDashboardRepo)(nil)

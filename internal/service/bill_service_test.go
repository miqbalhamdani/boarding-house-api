package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// stubBillRepo is an in-memory BillRepository for unit tests. It records the
// bills the service tries to insert and simulates the
// unique(room_assignment_id, billing_month) idempotency guard via seenKeys.
type stubBillRepo struct {
	activeAssignments []*model.RoomAssignment
	activeErr         error

	listResult *model.ListBillsResult
	listOwner  string // records the owner_id List was called with (isolation check)
	getResult  *model.Bill
	getErr     error

	inserted  []model.Bill
	seenKeys  map[string]bool // room_assignment_id|billing_month already billed
	insertErr error

	overdueOwner   string
	overdueToday   time.Time
	overdueUpdated int
	overdueErr     error
}

func (s *stubBillRepo) List(_ context.Context, ownerID string, _ model.ListBillsFilter) (*model.ListBillsResult, error) {
	s.listOwner = ownerID
	return s.listResult, nil
}

func (s *stubBillRepo) GetByID(_ context.Context, _, _ string) (*model.Bill, error) {
	return s.getResult, s.getErr
}

func (s *stubBillRepo) ActiveAssignments(_ context.Context, _ string) ([]*model.RoomAssignment, error) {
	return s.activeAssignments, s.activeErr
}

func (s *stubBillRepo) BeginTx(context.Context) (pgx.Tx, error) { return fakeTx{}, nil }

func (s *stubBillRepo) InsertBillIfAbsent(_ context.Context, _ pgx.Tx, b model.Bill) (bool, error) {
	if s.insertErr != nil {
		return false, s.insertErr
	}
	s.inserted = append(s.inserted, b)
	key := b.RoomAssignmentID + "|" + b.BillingMonth
	if s.seenKeys[key] {
		return false, nil // duplicate: skipped (BR-016)
	}
	if s.seenKeys == nil {
		s.seenKeys = map[string]bool{}
	}
	s.seenKeys[key] = true
	return true, nil
}

func (s *stubBillRepo) MarkOverdue(_ context.Context, ownerID string, today time.Time) (int, error) {
	s.overdueOwner = ownerID
	s.overdueToday = today
	return s.overdueUpdated, s.overdueErr
}

func activeAssignment(id string, dueDay, rent int) *model.RoomAssignment {
	return &model.RoomAssignment{
		ID:            id,
		OwnerID:       "owner-1",
		TenantID:      "tenant-" + id,
		RoomID:        "room-" + id,
		MonthlyRent:   rent,
		PaymentDueDay: dueDay,
		Status:        "active",
	}
}

func TestGenerateMonthly_CreatesOneBillPerActiveAssignment(t *testing.T) {
	repo := &stubBillRepo{activeAssignments: []*model.RoomAssignment{
		activeAssignment("a1", 10, 2000000),
		activeAssignment("a2", 5, 1500000),
	}}
	svc := NewBillService(repo)

	res, err := svc.GenerateMonthly(context.Background(), "owner-1", model.GenerateMonthlyInput{BillingMonth: "2026-07"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Created != 2 || res.Skipped != 0 || res.ActiveAssignment != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}

	// BR-012: amount is the assignment rent snapshot; bill is unpaid rent.
	b := repo.inserted[0]
	if b.Amount != 2000000 || b.Status != "unpaid" || b.BillType != "rent" {
		t.Fatalf("unexpected first bill: %+v", b)
	}
	if b.BillingMonth != "2026-07" || b.OwnerID != "owner-1" || b.RoomAssignmentID != "a1" {
		t.Fatalf("unexpected first bill linkage: %+v", b)
	}
	// Full-month period with due date on the payment_due_day.
	if got := b.BillingPeriodStart.Format(dateLayout); got != "2026-07-01" {
		t.Fatalf("period start: want 2026-07-01 got %s", got)
	}
	if got := b.BillingPeriodEnd.Format(dateLayout); got != "2026-07-31" {
		t.Fatalf("period end: want 2026-07-31 got %s", got)
	}
	if got := b.DueDate.Format(dateLayout); got != "2026-07-10" {
		t.Fatalf("due date: want 2026-07-10 got %s", got)
	}
}

func TestGenerateMonthly_Idempotent(t *testing.T) {
	// BR-016: a second run for the same month must not create duplicate bills.
	repo := &stubBillRepo{activeAssignments: []*model.RoomAssignment{
		activeAssignment("a1", 10, 2000000),
	}}
	svc := NewBillService(repo)

	first, err := svc.GenerateMonthly(context.Background(), "owner-1", model.GenerateMonthlyInput{BillingMonth: "2026-07"})
	if err != nil || first.Created != 1 {
		t.Fatalf("first run: %+v err=%v", first, err)
	}
	second, err := svc.GenerateMonthly(context.Background(), "owner-1", model.GenerateMonthlyInput{BillingMonth: "2026-07"})
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if second.Created != 0 || second.Skipped != 1 {
		t.Fatalf("second run should skip all: %+v", second)
	}
}

func TestGenerateMonthly_DefaultsToCurrentMonth(t *testing.T) {
	repo := &stubBillRepo{activeAssignments: []*model.RoomAssignment{activeAssignment("a1", 10, 2000000)}}
	svc := &billService{repo: repo, now: func() time.Time {
		return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	}}

	res, err := svc.GenerateMonthly(context.Background(), "owner-1", model.GenerateMonthlyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.BillingMonth != "2026-09" || repo.inserted[0].BillingMonth != "2026-09" {
		t.Fatalf("want current month 2026-09, got %+v", res)
	}
}

func TestGenerateMonthly_InvalidMonthRejected(t *testing.T) {
	svc := NewBillService(&stubBillRepo{})
	if _, err := svc.GenerateMonthly(context.Background(), "owner-1", model.GenerateMonthlyInput{BillingMonth: "2026/07"}); !errors.Is(err, ErrInvalidBillingMonth) {
		t.Fatalf("want ErrInvalidBillingMonth, got %v", err)
	}
}

func TestGenerateMonthly_NoActiveAssignments(t *testing.T) {
	repo := &stubBillRepo{activeAssignments: nil}
	svc := NewBillService(repo)

	res, err := svc.GenerateMonthly(context.Background(), "owner-1", model.GenerateMonthlyInput{BillingMonth: "2026-07"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Created != 0 || res.ActiveAssignment != 0 || len(repo.inserted) != 0 {
		t.Fatalf("expected no bills generated: %+v", res)
	}
}

func TestMarkOverdue_PassesTruncatedTodayAndOwner(t *testing.T) {
	repo := &stubBillRepo{overdueUpdated: 3}
	svc := &billService{repo: repo, now: func() time.Time {
		return time.Date(2026, 6, 27, 14, 30, 0, 0, time.UTC)
	}}

	res, err := svc.MarkOverdue(context.Background(), "owner-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Updated != 3 {
		t.Fatalf("want 3 updated, got %d", res.Updated)
	}
	if repo.overdueOwner != "owner-1" {
		t.Fatalf("owner isolation: MarkOverdue called with %q", repo.overdueOwner)
	}
	// Date is truncated to midnight UTC so the SQL compares due_date < today.
	if !repo.overdueToday.Equal(time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("today not truncated to midnight: %v", repo.overdueToday)
	}
}

func TestList_DerivesOwnerFromArgument(t *testing.T) {
	// Owner isolation (BR-001): the service must forward the authenticated
	// owner_id to the repository unchanged.
	repo := &stubBillRepo{listResult: &model.ListBillsResult{Bills: []*model.Bill{}}}
	svc := NewBillService(repo)

	if _, err := svc.List(context.Background(), "owner-42", model.ListBillsFilter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listOwner != "owner-42" {
		t.Fatalf("List used wrong owner_id: %q", repo.listOwner)
	}
}

func TestMonthlyBillingPeriod_DueDayClamped(t *testing.T) {
	tests := []struct {
		name      string
		month     string
		dueDay    int
		wantStart string
		wantEnd   string
		wantDue   string
	}{
		{"mid-month", "2026-07", 10, "2026-07-01", "2026-07-31", "2026-07-10"},
		{"clamp-feb-non-leap", "2026-02", 31, "2026-02-01", "2026-02-28", "2026-02-28"},
		{"leap-feb", "2024-02", 29, "2024-02-01", "2024-02-29", "2024-02-29"},
		{"december-rollover", "2026-12", 15, "2026-12-01", "2026-12-31", "2026-12-15"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			monthStart, _ := time.ParseInLocation(monthLayout, tc.month, time.UTC)
			p := monthlyBillingPeriod(monthStart, tc.dueDay)
			if got := p.Start.Format(dateLayout); got != tc.wantStart {
				t.Errorf("start: want %s got %s", tc.wantStart, got)
			}
			if got := p.End.Format(dateLayout); got != tc.wantEnd {
				t.Errorf("end: want %s got %s", tc.wantEnd, got)
			}
			if got := p.DueDate.Format(dateLayout); got != tc.wantDue {
				t.Errorf("due: want %s got %s", tc.wantDue, got)
			}
		})
	}
}

// Ensure the stub satisfies the repository interface.
var _ repository.BillRepository = (*stubBillRepo)(nil)

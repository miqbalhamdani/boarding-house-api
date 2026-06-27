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

type statusUpdate struct {
	id     string
	owner  string
	status string
}

// stubOnboardingRepo is a configurable in-memory OnboardingRepository for unit
// tests. It records the writes the service performs so assertions can verify
// the side effects (statuses, bill fields) without a real database.
type stubOnboardingRepo struct {
	roomStatus      string
	roomStatusErr   error
	tenantStatus    string
	tenantStatusErr error
	roomCount       int
	roomCountErr    error
	tenantCount     int
	tenantCountErr  error
	createAssignErr error
	createBillErr   error

	assignment    *model.RoomAssignment // returned by AssignmentForUpdate (cancel flow)
	assignmentErr error

	// recorded side effects
	assignInput   model.RoomAssignment
	billInput     model.Bill
	roomUpdates   []statusUpdate
	tenantUpdates []statusUpdate
	assignUpdates []statusUpdate
	billsCanceled []string
}

func (s *stubOnboardingRepo) BeginTx(context.Context) (pgx.Tx, error) { return fakeTx{}, nil }

func (s *stubOnboardingRepo) RoomStatusForUpdate(_ context.Context, _ pgx.Tx, _, _ string) (string, error) {
	return s.roomStatus, s.roomStatusErr
}

func (s *stubOnboardingRepo) TenantStatusForUpdate(_ context.Context, _ pgx.Tx, _, _ string) (string, error) {
	return s.tenantStatus, s.tenantStatusErr
}

func (s *stubOnboardingRepo) CountActiveAssignmentsForRoom(_ context.Context, _ pgx.Tx, _, _ string) (int, error) {
	return s.roomCount, s.roomCountErr
}

func (s *stubOnboardingRepo) CountActiveAssignmentsForTenant(_ context.Context, _ pgx.Tx, _, _ string) (int, error) {
	return s.tenantCount, s.tenantCountErr
}

func (s *stubOnboardingRepo) CreateAssignment(_ context.Context, _ pgx.Tx, a model.RoomAssignment) (*model.RoomAssignment, error) {
	if s.createAssignErr != nil {
		return nil, s.createAssignErr
	}
	s.assignInput = a
	a.ID = "ra-1"
	return &a, nil
}

func (s *stubOnboardingRepo) CreateBill(_ context.Context, _ pgx.Tx, b model.Bill) (*model.Bill, error) {
	if s.createBillErr != nil {
		return nil, s.createBillErr
	}
	s.billInput = b
	b.ID = "bill-1"
	return &b, nil
}

func (s *stubOnboardingRepo) UpdateRoomStatus(_ context.Context, _ pgx.Tx, roomID, ownerID, status string) error {
	s.roomUpdates = append(s.roomUpdates, statusUpdate{roomID, ownerID, status})
	return nil
}

func (s *stubOnboardingRepo) UpdateTenantStatus(_ context.Context, _ pgx.Tx, tenantID, ownerID, status string) error {
	s.tenantUpdates = append(s.tenantUpdates, statusUpdate{tenantID, ownerID, status})
	return nil
}

func (s *stubOnboardingRepo) AssignmentForUpdate(_ context.Context, _ pgx.Tx, _, _ string) (*model.RoomAssignment, error) {
	return s.assignment, s.assignmentErr
}

func (s *stubOnboardingRepo) UpdateAssignmentStatus(_ context.Context, _ pgx.Tx, id, ownerID, status string) error {
	s.assignUpdates = append(s.assignUpdates, statusUpdate{id, ownerID, status})
	return nil
}

func (s *stubOnboardingRepo) CancelUnpaidBillsForAssignment(_ context.Context, _ pgx.Tx, assignmentID, _ string) error {
	s.billsCanceled = append(s.billsCanceled, assignmentID)
	return nil
}

func validAssignInput() model.AssignRoomInput {
	return model.AssignRoomInput{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		RoomID:        "22222222-2222-2222-2222-222222222222",
		StartDate:     "2026-07-10",
		MonthlyRent:   2000000,
		PaymentDueDay: 10,
	}
}

func TestAssignRoom_Success(t *testing.T) {
	repo := &stubOnboardingRepo{roomStatus: "available", tenantStatus: "pending_payment"}
	svc := NewOnboardingService(repo)

	res, err := svc.AssignRoom(context.Background(), "owner-1", validAssignInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RoomAssignmentID != "ra-1" || res.FirstBillID != "bill-1" {
		t.Fatalf("unexpected ids: %+v", res)
	}
	if res.RoomStatus != "reserved" || res.TenantStatus != "pending_payment" {
		t.Fatalf("unexpected statuses: %+v", res)
	}

	// Room reserved (BR-005) and tenant set to pending_payment (BR-007).
	if len(repo.roomUpdates) != 1 || repo.roomUpdates[0].status != "reserved" {
		t.Fatalf("room not reserved: %+v", repo.roomUpdates)
	}
	if repo.roomUpdates[0].owner != "owner-1" {
		t.Fatalf("room update used wrong owner: %+v", repo.roomUpdates[0])
	}
	if len(repo.tenantUpdates) != 1 || repo.tenantUpdates[0].status != "pending_payment" {
		t.Fatalf("tenant not pending_payment: %+v", repo.tenantUpdates)
	}

	// First bill (BR-014) carries the assignment rent and is unpaid.
	if repo.billInput.Amount != 2000000 || repo.billInput.Status != "unpaid" {
		t.Fatalf("unexpected bill: %+v", repo.billInput)
	}
	if repo.billInput.BillingMonth != "2026-07" || repo.billInput.RoomAssignmentID != "ra-1" {
		t.Fatalf("unexpected bill linkage: %+v", repo.billInput)
	}
	if repo.assignInput.OwnerID != "owner-1" || repo.assignInput.Status != "pending_payment" {
		t.Fatalf("unexpected assignment: %+v", repo.assignInput)
	}
}

func TestAssignRoom_InvalidStartDate(t *testing.T) {
	svc := NewOnboardingService(&stubOnboardingRepo{roomStatus: "available"})
	in := validAssignInput()
	in.StartDate = "10-07-2026"

	if _, err := svc.AssignRoom(context.Background(), "owner-1", in); !errors.Is(err, ErrInvalidStartDate) {
		t.Fatalf("want ErrInvalidStartDate, got %v", err)
	}
}

func TestAssignRoom_RoomNotAvailable(t *testing.T) {
	svc := NewOnboardingService(&stubOnboardingRepo{roomStatus: "occupied"})

	if _, err := svc.AssignRoom(context.Background(), "owner-1", validAssignInput()); !errors.Is(err, ErrRoomNotAvailable) {
		t.Fatalf("want ErrRoomNotAvailable, got %v", err)
	}
}

func TestAssignRoom_CrossOwnerRoomNotFound(t *testing.T) {
	// Room belongs to another owner: the owner-scoped lookup returns ErrNotFound.
	svc := NewOnboardingService(&stubOnboardingRepo{roomStatusErr: repository.ErrNotFound})

	if _, err := svc.AssignRoom(context.Background(), "owner-1", validAssignInput()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-owner room, got %v", err)
	}
}

func TestAssignRoom_RoomHasActiveAssignment(t *testing.T) {
	svc := NewOnboardingService(&stubOnboardingRepo{roomStatus: "available", roomCount: 1})

	if _, err := svc.AssignRoom(context.Background(), "owner-1", validAssignInput()); !errors.Is(err, ErrRoomHasActiveAssignment) {
		t.Fatalf("want ErrRoomHasActiveAssignment, got %v", err)
	}
}

func TestAssignRoom_TenantHasActiveAssignment(t *testing.T) {
	svc := NewOnboardingService(&stubOnboardingRepo{roomStatus: "available", tenantCount: 1})

	if _, err := svc.AssignRoom(context.Background(), "owner-1", validAssignInput()); !errors.Is(err, ErrTenantHasActiveAssignment) {
		t.Fatalf("want ErrTenantHasActiveAssignment, got %v", err)
	}
}

func TestAssignRoom_RaceUniqueViolationMapped(t *testing.T) {
	// Concurrent insert wins the partial unique index; repo surfaces the typed
	// error which the service maps to its tenant-conflict domain error.
	svc := NewOnboardingService(&stubOnboardingRepo{
		roomStatus:      "available",
		createAssignErr: repository.ErrTenantAssignmentExists,
	})

	if _, err := svc.AssignRoom(context.Background(), "owner-1", validAssignInput()); !errors.Is(err, ErrTenantHasActiveAssignment) {
		t.Fatalf("want ErrTenantHasActiveAssignment, got %v", err)
	}
}

func TestAssignRoom_DuplicateBillPropagated(t *testing.T) {
	svc := NewOnboardingService(&stubOnboardingRepo{
		roomStatus:    "available",
		createBillErr: repository.ErrDuplicateBill,
	})

	if _, err := svc.AssignRoom(context.Background(), "owner-1", validAssignInput()); !errors.Is(err, repository.ErrDuplicateBill) {
		t.Fatalf("want ErrDuplicateBill, got %v", err)
	}
}

func TestCancel_Success(t *testing.T) {
	repo := &stubOnboardingRepo{
		assignment: &model.RoomAssignment{
			ID: "ra-1", OwnerID: "owner-1", TenantID: "t-1", RoomID: "r-1", Status: "pending_payment",
		},
	}
	svc := NewOnboardingService(repo)

	if err := svc.Cancel(context.Background(), "owner-1", "ra-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.assignUpdates) != 1 || repo.assignUpdates[0].status != "cancelled" {
		t.Fatalf("assignment not cancelled: %+v", repo.assignUpdates)
	}
	if len(repo.billsCanceled) != 1 || repo.billsCanceled[0] != "ra-1" {
		t.Fatalf("bills not cancelled: %+v", repo.billsCanceled)
	}
	if len(repo.roomUpdates) != 1 || repo.roomUpdates[0].status != "available" {
		t.Fatalf("room not released: %+v", repo.roomUpdates)
	}
}

func TestCancel_NotPendingRejected(t *testing.T) {
	repo := &stubOnboardingRepo{
		assignment: &model.RoomAssignment{ID: "ra-1", OwnerID: "owner-1", Status: "active"},
	}
	svc := NewOnboardingService(repo)

	if err := svc.Cancel(context.Background(), "owner-1", "ra-1"); !errors.Is(err, ErrOnboardingNotCancelable) {
		t.Fatalf("want ErrOnboardingNotCancelable, got %v", err)
	}
	if len(repo.assignUpdates) != 0 {
		t.Fatalf("expected no writes for non-cancelable assignment, got %+v", repo.assignUpdates)
	}
}

func TestCancel_CrossOwnerNotFound(t *testing.T) {
	svc := NewOnboardingService(&stubOnboardingRepo{assignmentErr: repository.ErrNotFound})

	if err := svc.Cancel(context.Background(), "owner-1", "ra-other"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-owner cancel, got %v", err)
	}
}

func TestComputeBillingPeriod(t *testing.T) {
	tests := []struct {
		name      string
		start     string
		dueDay    int
		wantMonth string
		wantEnd   string
		wantDue   string
	}{
		{"mid-month", "2026-07-10", 10, "2026-07", "2026-07-31", "2026-07-10"},
		{"due-day-clamped-feb", "2026-02-05", 31, "2026-02", "2026-02-28", "2026-02-28"},
		{"leap-february", "2024-02-01", 29, "2024-02", "2024-02-29", "2024-02-29"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, _ := time.Parse(dateLayout, tc.start)
			p := computeBillingPeriod(start, tc.dueDay)
			if p.BillingMonth != tc.wantMonth {
				t.Errorf("month: want %s got %s", tc.wantMonth, p.BillingMonth)
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

func TestBuildBillNumber(t *testing.T) {
	got := buildBillNumber("2026-07", "abcdef12-3456-7890-abcd-ef1234567890")
	want := "INV-2026-07-ABCDEF12"
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

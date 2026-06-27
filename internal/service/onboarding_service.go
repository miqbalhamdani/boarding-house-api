package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// Domain errors for the onboarding flow. Handlers map these to HTTP responses.
var (
	ErrInvalidStartDate          = errors.New("start_date must be a valid date (YYYY-MM-DD)")
	ErrRoomNotAvailable          = errors.New("room is not available for assignment")
	ErrRoomHasActiveAssignment   = errors.New("room already has an active or pending assignment")
	ErrTenantHasActiveAssignment = errors.New("tenant already has an active or pending assignment")
	ErrOnboardingNotCancelable   = errors.New("only a pending_payment onboarding can be cancelled")
)

const dateLayout = "2006-01-02"

// OnboardingService assigns tenants to rooms and creates the first rent bill.
type OnboardingService interface {
	AssignRoom(ctx context.Context, ownerID string, in model.AssignRoomInput) (*model.AssignRoomResult, error)
	Cancel(ctx context.Context, ownerID, assignmentID string) error
}

type onboardingService struct {
	repo repository.OnboardingRepository
}

// NewOnboardingService wires an OnboardingService to its repository.
func NewOnboardingService(repo repository.OnboardingRepository) OnboardingService {
	return &onboardingService{repo: repo}
}

func (s *onboardingService) AssignRoom(ctx context.Context, ownerID string, in model.AssignRoomInput) (result *model.AssignRoomResult, err error) {
	startDate, perr := time.Parse(dateLayout, in.StartDate)
	if perr != nil {
		return nil, ErrInvalidStartDate
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	// Roll back unless we explicitly commit. Safe to call after a commit.
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the room and verify it belongs to the owner and is available (BR-004).
	roomStatus, err := s.repo.RoomStatusForUpdate(ctx, tx, in.RoomID, ownerID)
	if err != nil {
		return nil, err
	}
	if roomStatus != "available" {
		return nil, ErrRoomNotAvailable
	}

	// Lock the tenant and verify it belongs to the owner.
	if _, err = s.repo.TenantStatusForUpdate(ctx, tx, in.TenantID, ownerID); err != nil {
		return nil, err
	}

	// BR-010 / BR-011: no existing active or pending assignment for room or tenant.
	roomCount, err := s.repo.CountActiveAssignmentsForRoom(ctx, tx, in.RoomID, ownerID)
	if err != nil {
		return nil, err
	}
	if roomCount > 0 {
		return nil, ErrRoomHasActiveAssignment
	}
	tenantCount, err := s.repo.CountActiveAssignmentsForTenant(ctx, tx, in.TenantID, ownerID)
	if err != nil {
		return nil, err
	}
	if tenantCount > 0 {
		return nil, ErrTenantHasActiveAssignment
	}

	// Create the room assignment (status pending_payment).
	assignment, err := s.repo.CreateAssignment(ctx, tx, model.RoomAssignment{
		OwnerID:       ownerID,
		TenantID:      in.TenantID,
		RoomID:        in.RoomID,
		StartDate:     startDate,
		MonthlyRent:   in.MonthlyRent,
		PaymentDueDay: in.PaymentDueDay,
		Status:        "pending_payment",
	})
	if err != nil {
		return nil, mapAssignmentConflict(err)
	}

	// Create the first rent bill (BR-014).
	period := computeBillingPeriod(startDate, in.PaymentDueDay)
	bill, err := s.repo.CreateBill(ctx, tx, model.Bill{
		OwnerID:            ownerID,
		TenantID:           in.TenantID,
		RoomID:             in.RoomID,
		RoomAssignmentID:   assignment.ID,
		BillNumber:         buildBillNumber(period.BillingMonth, assignment.ID),
		BillType:           "rent",
		BillingMonth:       period.BillingMonth,
		BillingPeriodStart: period.Start,
		BillingPeriodEnd:   period.End,
		Amount:             in.MonthlyRent,
		DueDate:            period.DueDate,
		Status:             "unpaid",
	})
	if err != nil {
		return nil, err
	}

	// Reserve the room (BR-005) and set tenant to pending_payment (BR-007).
	if err = s.repo.UpdateRoomStatus(ctx, tx, in.RoomID, ownerID, "reserved"); err != nil {
		return nil, err
	}
	if err = s.repo.UpdateTenantStatus(ctx, tx, in.TenantID, ownerID, "pending_payment"); err != nil {
		return nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit onboarding: %w", err)
	}

	return &model.AssignRoomResult{
		RoomAssignmentID: assignment.ID,
		FirstBillID:      bill.ID,
		TenantStatus:     "pending_payment",
		RoomStatus:       "reserved",
	}, nil
}

func (s *onboardingService) Cancel(ctx context.Context, ownerID, assignmentID string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	assignment, err := s.repo.AssignmentForUpdate(ctx, tx, assignmentID, ownerID)
	if err != nil {
		return err
	}
	// Only an onboarding that has not yet been paid can be cancelled here.
	if assignment.Status != "pending_payment" {
		return ErrOnboardingNotCancelable
	}

	if err = s.repo.UpdateAssignmentStatus(ctx, tx, assignmentID, ownerID, "cancelled"); err != nil {
		return err
	}
	if err = s.repo.CancelUnpaidBillsForAssignment(ctx, tx, assignmentID, ownerID); err != nil {
		return err
	}
	// Release the room back to available and revert the tenant to its
	// pre-onboarding state.
	if err = s.repo.UpdateRoomStatus(ctx, tx, assignment.RoomID, ownerID, "available"); err != nil {
		return err
	}
	if err = s.repo.UpdateTenantStatus(ctx, tx, assignment.TenantID, ownerID, "pending_payment"); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cancel onboarding: %w", err)
	}
	return nil
}

// billingPeriod holds the derived fields for the first rent bill.
type billingPeriod struct {
	BillingMonth string
	Start        time.Time
	End          time.Time
	DueDate      time.Time
}

// computeBillingPeriod derives the first bill's month, period and due date from
// the assignment start date and payment due day. The first billing period runs
// from the start date to the end of that calendar month; the due date is the
// payment_due_day of the start month, clamped to the last day of the month.
func computeBillingPeriod(start time.Time, dueDay int) billingPeriod {
	year, month := start.Year(), start.Month()
	loc := start.Location()

	// First day of the next month, then step back one day for the month end.
	monthEnd := time.Date(year, month+1, 1, 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	daysInMonth := monthEnd.Day()
	if dueDay > daysInMonth {
		dueDay = daysInMonth
	}

	return billingPeriod{
		BillingMonth: start.Format("2006-01"),
		Start:        start,
		End:          monthEnd,
		DueDate:      time.Date(year, month, dueDay, 0, 0, 0, 0, loc),
	}
}

// buildBillNumber produces a per-owner-unique, human-readable bill number.
func buildBillNumber(billingMonth, assignmentID string) string {
	suffix := strings.ReplaceAll(assignmentID, "-", "")
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return fmt.Sprintf("INV-%s-%s", billingMonth, strings.ToUpper(suffix))
}

// mapAssignmentConflict normalises the repository's race-condition unique
// violations onto the service-level domain errors.
func mapAssignmentConflict(err error) error {
	switch {
	case errors.Is(err, repository.ErrRoomAssignmentExists):
		return ErrRoomHasActiveAssignment
	case errors.Is(err, repository.ErrTenantAssignmentExists):
		return ErrTenantHasActiveAssignment
	default:
		return err
	}
}

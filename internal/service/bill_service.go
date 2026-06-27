package service

import (
	"context"
	"errors"
	"time"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// ErrInvalidBillingMonth is returned when billing_month is not a valid YYYY-MM.
var ErrInvalidBillingMonth = errors.New("billing_month must be a valid month (YYYY-MM)")

const monthLayout = "2006-01"

// BillService implements the monthly billing use cases (Module 05).
type BillService interface {
	List(ctx context.Context, ownerID string, f model.ListBillsFilter) (*model.ListBillsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Bill, error)
	GenerateMonthly(ctx context.Context, ownerID string, in model.GenerateMonthlyInput) (*model.GenerateMonthlyResult, error)
	MarkOverdue(ctx context.Context, ownerID string) (*model.MarkOverdueResult, error)
}

type billService struct {
	repo repository.BillRepository
	now  func() time.Time
}

// NewBillService wires a BillService to its repository.
func NewBillService(repo repository.BillRepository) BillService {
	return &billService{repo: repo, now: time.Now}
}

func (s *billService) List(ctx context.Context, ownerID string, f model.ListBillsFilter) (*model.ListBillsResult, error) {
	return s.repo.List(ctx, ownerID, f)
}

func (s *billService) GetByID(ctx context.Context, id, ownerID string) (*model.Bill, error) {
	return s.repo.GetByID(ctx, id, ownerID)
}

// GenerateMonthly creates one rent bill per active assignment for the target
// month. It is idempotent (BR-015, BR-016): the database's
// unique(room_assignment_id, billing_month) constraint means a re-run for the
// same month creates no duplicates, so both the daily scheduler and the manual
// backup action are safe to call repeatedly.
func (s *billService) GenerateMonthly(ctx context.Context, ownerID string, in model.GenerateMonthlyInput) (*model.GenerateMonthlyResult, error) {
	month := in.BillingMonth
	if month == "" {
		month = s.now().UTC().Format(monthLayout)
	}
	monthStart, err := time.ParseInLocation(monthLayout, month, time.UTC)
	if err != nil {
		return nil, ErrInvalidBillingMonth
	}

	assignments, err := s.repo.ActiveAssignments(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	result := &model.GenerateMonthlyResult{
		BillingMonth:     month,
		ActiveAssignment: len(assignments),
	}

	// Generate the whole month's bills atomically. Re-runs remain idempotent
	// because each insert is ON CONFLICT DO NOTHING against
	// unique(room_assignment_id, billing_month).
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, a := range assignments {
		period := monthlyBillingPeriod(monthStart, a.PaymentDueDay)
		created, err := s.repo.InsertBillIfAbsent(ctx, tx, model.Bill{
			OwnerID:            ownerID,
			TenantID:           a.TenantID,
			RoomID:             a.RoomID,
			RoomAssignmentID:   a.ID,
			BillNumber:         buildBillNumber(period.BillingMonth, a.ID),
			BillType:           "rent",
			BillingMonth:       period.BillingMonth,
			BillingPeriodStart: period.Start,
			BillingPeriodEnd:   period.End,
			Amount:             a.MonthlyRent, // BR-012: rent snapshot from the assignment.
			DueDate:            period.DueDate,
			Status:             "unpaid",
		})
		if err != nil {
			return nil, err
		}
		if created {
			result.Created++
		} else {
			result.Skipped++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

// MarkOverdue flips the owner's unpaid, past-due bills to overdue (BR-018).
func (s *billService) MarkOverdue(ctx context.Context, ownerID string) (*model.MarkOverdueResult, error) {
	today := s.now().UTC().Truncate(24 * time.Hour)
	updated, err := s.repo.MarkOverdue(ctx, ownerID, today)
	if err != nil {
		return nil, err
	}
	return &model.MarkOverdueResult{Updated: updated}, nil
}

// monthlyBillingPeriod derives a full-calendar-month rent period and due date.
// The period spans the whole month; the due date is payment_due_day clamped to
// the last day of the month (so day 31 in February falls on the 28th/29th).
func monthlyBillingPeriod(monthStart time.Time, dueDay int) billingPeriod {
	year, month, loc := monthStart.Year(), monthStart.Month(), monthStart.Location()

	monthEnd := time.Date(year, month+1, 1, 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	if daysInMonth := monthEnd.Day(); dueDay > daysInMonth {
		dueDay = daysInMonth
	}

	return billingPeriod{
		BillingMonth: monthStart.Format(monthLayout),
		Start:        monthStart,
		End:          monthEnd,
		DueDate:      time.Date(year, month, dueDay, 0, 0, 0, 0, loc),
	}
}

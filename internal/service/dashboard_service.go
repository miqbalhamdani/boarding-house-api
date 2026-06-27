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

// DashboardService implements the owner dashboard use case (Module 07).
type DashboardService interface {
	// Summary returns owner-scoped overview metrics for the given month. An
	// empty month defaults to the current calendar month (UTC).
	Summary(ctx context.Context, ownerID, month string) (*model.DashboardSummary, error)
}

type dashboardService struct {
	repo repository.DashboardRepository
	now  func() time.Time
}

// NewDashboardService wires a DashboardService to its repository.
func NewDashboardService(repo repository.DashboardRepository) DashboardService {
	return &dashboardService{repo: repo, now: time.Now}
}

func (s *dashboardService) Summary(ctx context.Context, ownerID, month string) (*model.DashboardSummary, error) {
	if month == "" {
		month = s.now().UTC().Format(monthLayout)
	}
	monthStart, err := time.ParseInLocation(monthLayout, month, time.UTC)
	if err != nil {
		return nil, ErrInvalidMonth
	}
	// Half-open month window [monthStart, monthEnd) for payment_date filtering.
	monthEnd := monthStart.AddDate(0, 1, 0)

	return s.repo.Summary(ctx, ownerID, monthStart, monthEnd)
}

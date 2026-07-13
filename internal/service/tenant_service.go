package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// TenantService holds tenant management business logic.
type TenantService interface {
	Create(ctx context.Context, ownerID string, in model.CreateTenantInput) (*model.Tenant, error)
	List(ctx context.Context, ownerID string, f model.ListTenantsFilter) (*model.ListTenantsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Tenant, error)
	// GetDetail returns a tenant together with their current room (if any) and
	// paginated bill history.
	GetDetail(ctx context.Context, id, ownerID string, billFilter model.ListBillsFilter) (*model.TenantDetailView, error)
	Update(ctx context.Context, id, ownerID string, in model.UpdateTenantInput) (*model.Tenant, error)
	Delete(ctx context.Context, id, ownerID string) error
}

type tenantService struct {
	repo     repository.TenantRepository
	billRepo repository.BillRepository
}

// NewTenantService wires a TenantService to its repositories.
func NewTenantService(repo repository.TenantRepository, billRepo repository.BillRepository) TenantService {
	return &tenantService{repo: repo, billRepo: billRepo}
}

func (s *tenantService) Create(ctx context.Context, ownerID string, in model.CreateTenantInput) (*model.Tenant, error) {
	// A tenant is always created as pending_payment; it becomes active only after
	// the first rent bill is paid (BR-008). Status is never accepted from input.
	var passwordHash *string
	if in.Password != "" {
		h, err := hashPassword(in.Password)
		if err != nil {
			return nil, err
		}
		passwordHash = &h
	}
	return s.repo.Create(ctx, ownerID, in, passwordHash)
}

func (s *tenantService) List(ctx context.Context, ownerID string, f model.ListTenantsFilter) (*model.ListTenantsResult, error) {
	return s.repo.List(ctx, ownerID, f)
}

func (s *tenantService) GetByID(ctx context.Context, id, ownerID string) (*model.Tenant, error) {
	return s.repo.GetByID(ctx, id, ownerID)
}

func (s *tenantService) GetDetail(ctx context.Context, id, ownerID string, billFilter model.ListBillsFilter) (*model.TenantDetailView, error) {
	tenant, err := s.repo.GetByID(ctx, id, ownerID)
	if err != nil {
		return nil, err
	}

	currentRoom, err := s.repo.GetCurrentRoom(ctx, id, ownerID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if errors.Is(err, repository.ErrNotFound) {
		currentRoom = nil
	}

	billFilter.TenantID = id
	billHistory, err := s.billRepo.List(ctx, ownerID, billFilter)
	if err != nil {
		return nil, err
	}

	return &model.TenantDetailView{Tenant: tenant, CurrentRoom: currentRoom, BillHistory: billHistory}, nil
}

func (s *tenantService) Update(ctx context.Context, id, ownerID string, in model.UpdateTenantInput) (*model.Tenant, error) {
	var passwordHash *string
	if in.Password != nil && *in.Password != "" {
		h, err := hashPassword(*in.Password)
		if err != nil {
			return nil, err
		}
		passwordHash = &h
	}
	return s.repo.Update(ctx, id, ownerID, in, passwordHash)
}

func (s *tenantService) Delete(ctx context.Context, id, ownerID string) error {
	// Confirm the tenant exists and belongs to this owner before soft deleting.
	if _, err := s.repo.GetByID(ctx, id, ownerID); err != nil {
		return err
	}
	return s.repo.SoftDelete(ctx, id, ownerID)
}

func hashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

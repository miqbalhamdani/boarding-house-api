package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/pkg/auth"
)

// ErrInvalidCredentials is a deliberately generic error so responses do not
// leak whether the email or the password was wrong.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInactiveUser is returned when a user exists but is not allowed to log in.
var ErrInactiveUser = errors.New("user is not active")

// AuthService holds authentication business logic for owners and tenants.
type AuthService interface {
	RegisterOwner(ctx context.Context, in model.RegisterOwnerInput) (*model.OwnerAuthResult, error)
	LoginOwner(ctx context.Context, in model.LoginInput) (*model.OwnerAuthResult, error)
	LoginTenant(ctx context.Context, in model.LoginInput) (*model.TenantAuthResult, error)
	RefreshOwner(ctx context.Context, refreshToken string) (*model.AuthTokens, error)
	RefreshTenant(ctx context.Context, refreshToken string) (*model.AuthTokens, error)
	GetTenantProfile(ctx context.Context, tenantID string) (*model.TenantAuth, error)
}

type authService struct {
	owners  repository.OwnerRepository
	tenants repository.TenantAuthRepository
	tokens  *auth.Manager
}

// NewAuthService wires an AuthService to its repositories and token manager.
func NewAuthService(owners repository.OwnerRepository, tenants repository.TenantAuthRepository, tokens *auth.Manager) AuthService {
	return &authService{owners: owners, tenants: tenants, tokens: tokens}
}

func (s *authService) RegisterOwner(ctx context.Context, in model.RegisterOwnerInput) (*model.OwnerAuthResult, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	tx, err := s.owners.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // no-op after commit

	owner, err := s.owners.CreateOwner(ctx, tx, in)
	if err != nil {
		return nil, err
	}

	user, err := s.owners.CreateOwnerUser(ctx, tx, owner.ID, in.FullName, in.Email, string(hash))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	access, refresh, err := s.tokens.GenerateOwnerTokens(user.ID, owner.ID)
	if err != nil {
		return nil, err
	}

	return &model.OwnerAuthResult{
		OwnerID:     owner.ID,
		OwnerUserID: user.ID,
		FullName:    user.FullName,
		Email:       user.Email,
		Tokens:      model.AuthTokens{AccessToken: access, RefreshToken: refresh},
	}, nil
}

func (s *authService) LoginOwner(ctx context.Context, in model.LoginInput) (*model.OwnerAuthResult, error) {
	user, err := s.owners.GetOwnerUserByEmail(ctx, in.Email)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		return nil, ErrInvalidCredentials
	}
	if user.Status != "active" {
		return nil, ErrInactiveUser
	}

	access, refresh, err := s.tokens.GenerateOwnerTokens(user.ID, user.OwnerID)
	if err != nil {
		return nil, err
	}

	return &model.OwnerAuthResult{
		OwnerID:     user.OwnerID,
		OwnerUserID: user.ID,
		FullName:    user.FullName,
		Email:       user.Email,
		Tokens:      model.AuthTokens{AccessToken: access, RefreshToken: refresh},
	}, nil
}

func (s *authService) LoginTenant(ctx context.Context, in model.LoginInput) (*model.TenantAuthResult, error) {
	tenant, err := s.tenants.GetByEmail(ctx, in.Email)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	// Tenant password is provisioned during onboarding (out of this module's scope).
	if tenant.PasswordHash == nil {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(*tenant.PasswordHash), []byte(in.Password)) != nil {
		return nil, ErrInvalidCredentials
	}

	access, refresh, err := s.tokens.GenerateTenantTokens(tenant.ID, tenant.OwnerID)
	if err != nil {
		return nil, err
	}

	return &model.TenantAuthResult{
		TenantID: tenant.ID,
		OwnerID:  tenant.OwnerID,
		FullName: tenant.FullName,
		Tokens:   model.AuthTokens{AccessToken: access, RefreshToken: refresh},
	}, nil
}

func (s *authService) RefreshOwner(ctx context.Context, refreshToken string) (*model.AuthTokens, error) {
	claims, err := s.tokens.ParseOwner(refreshToken, auth.TypeRefresh)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	access, refresh, err := s.tokens.GenerateOwnerTokens(claims.OwnerUserID, claims.OwnerID)
	if err != nil {
		return nil, err
	}
	return &model.AuthTokens{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *authService) RefreshTenant(ctx context.Context, refreshToken string) (*model.AuthTokens, error) {
	claims, err := s.tokens.ParseTenant(refreshToken, auth.TypeRefresh)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	access, refresh, err := s.tokens.GenerateTenantTokens(claims.TenantID, claims.OwnerID)
	if err != nil {
		return nil, err
	}
	return &model.AuthTokens{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *authService) GetTenantProfile(ctx context.Context, tenantID string) (*model.TenantAuth, error) {
	return s.tenants.GetByID(ctx, tenantID)
}

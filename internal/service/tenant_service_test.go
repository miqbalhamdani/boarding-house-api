package service

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

type stubTenantMgmtRepo struct {
	tenant       *model.Tenant
	createErr    error
	getErr       error
	updateErr    error
	deleteErr    error
	deletedID    string
	gotPassHash  *string
	createOwner  string
	getCalledFor string
}

func (s *stubTenantMgmtRepo) Create(_ context.Context, ownerID string, _ model.CreateTenantInput, passwordHash *string) (*model.Tenant, error) {
	s.createOwner = ownerID
	s.gotPassHash = passwordHash
	return s.tenant, s.createErr
}

func (s *stubTenantMgmtRepo) List(_ context.Context, _ string, f model.ListTenantsFilter) (*model.ListTenantsResult, error) {
	tenants := []*model.Tenant{}
	total := 0
	if s.tenant != nil {
		tenants = []*model.Tenant{s.tenant}
		total = 1
	}
	return &model.ListTenantsResult{Tenants: tenants, Total: total, Page: f.Page, Limit: f.Limit}, nil
}

func (s *stubTenantMgmtRepo) GetByID(_ context.Context, id, _ string) (*model.Tenant, error) {
	s.getCalledFor = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.tenant != nil {
		s.tenant.ID = id
	}
	return s.tenant, nil
}

func (s *stubTenantMgmtRepo) Update(_ context.Context, _ string, _ string, _ model.UpdateTenantInput, passwordHash *string) (*model.Tenant, error) {
	s.gotPassHash = passwordHash
	return s.tenant, s.updateErr
}

func (s *stubTenantMgmtRepo) SoftDelete(_ context.Context, id, _ string) error {
	s.deletedID = id
	return s.deleteErr
}

func TestCreateTenant_DefaultsAndNoCredentialWhenNoPassword(t *testing.T) {
	tenant := &model.Tenant{ID: "t-1", OwnerID: "o-1", FullName: "Budi", Status: "pending_payment"}
	stub := &stubTenantMgmtRepo{tenant: tenant}
	svc := NewTenantService(stub)

	got, err := svc.Create(context.Background(), "o-1", model.CreateTenantInput{FullName: "Budi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "pending_payment" {
		t.Fatalf("want status=pending_payment, got %q", got.Status)
	}
	if stub.createOwner != "o-1" {
		t.Fatalf("owner_id must be derived from caller, got %q", stub.createOwner)
	}
	if stub.gotPassHash != nil {
		t.Fatalf("expected no password hash when password omitted, got %v", *stub.gotPassHash)
	}
}

func TestCreateTenant_HashesPasswordWhenProvided(t *testing.T) {
	tenant := &model.Tenant{ID: "t-1", OwnerID: "o-1", FullName: "Budi", Status: "pending_payment"}
	stub := &stubTenantMgmtRepo{tenant: tenant}
	svc := NewTenantService(stub)

	_, err := svc.Create(context.Background(), "o-1", model.CreateTenantInput{FullName: "Budi", Password: "secret123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.gotPassHash == nil {
		t.Fatal("expected password to be hashed and passed to repository")
	}
	if *stub.gotPassHash == "secret123" {
		t.Fatal("password must be hashed, not stored in plaintext")
	}
	if bcrypt.CompareHashAndPassword([]byte(*stub.gotPassHash), []byte("secret123")) != nil {
		t.Fatal("stored hash does not match original password")
	}
}

func TestCreateTenant_EmailTaken(t *testing.T) {
	svc := NewTenantService(&stubTenantMgmtRepo{createErr: repository.ErrTenantEmailTaken})

	_, err := svc.Create(context.Background(), "o-1", model.CreateTenantInput{FullName: "Budi", Email: "budi@example.com"})
	if !errors.Is(err, repository.ErrTenantEmailTaken) {
		t.Fatalf("want ErrTenantEmailTaken, got %v", err)
	}
}

func TestGetTenant_CrossOwnerIsolated(t *testing.T) {
	// The repository filters by owner_id; cross-owner access surfaces as ErrNotFound.
	svc := NewTenantService(&stubTenantMgmtRepo{getErr: repository.ErrNotFound})

	_, err := svc.GetByID(context.Background(), "t-other", "o-1")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-owner access, got %v", err)
	}
}

func TestUpdateTenant_HashesPasswordWhenProvided(t *testing.T) {
	tenant := &model.Tenant{ID: "t-1", OwnerID: "o-1", FullName: "Budi", Status: "pending_payment"}
	stub := &stubTenantMgmtRepo{tenant: tenant}
	svc := NewTenantService(stub)

	pw := "newsecret"
	_, err := svc.Update(context.Background(), "t-1", "o-1", model.UpdateTenantInput{Password: &pw})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.gotPassHash == nil || bcrypt.CompareHashAndPassword([]byte(*stub.gotPassHash), []byte(pw)) != nil {
		t.Fatal("expected password to be hashed on update")
	}
}

func TestUpdateTenant_NoPasswordLeavesCredentialUntouched(t *testing.T) {
	tenant := &model.Tenant{ID: "t-1", OwnerID: "o-1", FullName: "Budi", Status: "pending_payment"}
	stub := &stubTenantMgmtRepo{tenant: tenant}
	svc := NewTenantService(stub)

	newName := "Budi Santoso"
	_, err := svc.Update(context.Background(), "t-1", "o-1", model.UpdateTenantInput{FullName: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.gotPassHash != nil {
		t.Fatalf("expected nil password hash when password not provided, got %v", *stub.gotPassHash)
	}
}

func TestDeleteTenant_NotFound(t *testing.T) {
	svc := NewTenantService(&stubTenantMgmtRepo{getErr: repository.ErrNotFound})

	if err := svc.Delete(context.Background(), "t-missing", "o-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteTenant_Succeeds(t *testing.T) {
	tenant := &model.Tenant{ID: "t-1", OwnerID: "o-1", FullName: "Budi", Status: "active"}
	stub := &stubTenantMgmtRepo{tenant: tenant}
	svc := NewTenantService(stub)

	if err := svc.Delete(context.Background(), "t-1", "o-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.deletedID != "t-1" {
		t.Fatalf("expected soft delete called with t-1, got %q", stub.deletedID)
	}
}

func TestListTenants_ReturnsOwnTenantsOnly(t *testing.T) {
	tenant := &model.Tenant{ID: "t-1", OwnerID: "o-1", FullName: "Budi", Status: "active"}
	svc := NewTenantService(&stubTenantMgmtRepo{tenant: tenant})

	result, err := svc.List(context.Background(), "o-1", model.ListTenantsFilter{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 || len(result.Tenants) != 1 {
		t.Fatalf("want 1 tenant, got total=%d tenants=%d", result.Total, len(result.Tenants))
	}
	if result.Tenants[0].OwnerID != "o-1" {
		t.Fatalf("tenant owner_id mismatch: %q", result.Tenants[0].OwnerID)
	}
}

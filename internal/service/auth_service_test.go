package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/pkg/auth"
)

// fakeTx satisfies pgx.Tx; only Commit/Rollback are exercised by the stub repo.
type fakeTx struct{ pgx.Tx }

func (fakeTx) Commit(context.Context) error   { return nil }
func (fakeTx) Rollback(context.Context) error { return nil }

type stubOwnerRepo struct {
	created    []string // emails passed to CreateOwnerUser
	emailTaken bool
	loginUser  *model.OwnerUser
	loginErr   error

	userByID    *model.OwnerUser
	userByIDErr error
	ownerByID   *model.Owner
	ownerErr    error
}

func (s *stubOwnerRepo) BeginTx(context.Context) (pgx.Tx, error) { return fakeTx{}, nil }

func (s *stubOwnerRepo) CreateOwner(_ context.Context, _ pgx.Tx, in model.RegisterOwnerInput) (*model.Owner, error) {
	if s.emailTaken {
		return nil, repository.ErrEmailTaken
	}
	return &model.Owner{ID: "owner-1", FullName: in.FullName, Email: in.Email}, nil
}

func (s *stubOwnerRepo) CreateOwnerUser(_ context.Context, _ pgx.Tx, ownerID, fullName, email, passwordHash string) (*model.OwnerUser, error) {
	s.created = append(s.created, email)
	// Verify the password was hashed, not stored raw.
	if passwordHash == "" || passwordHash == "password123" {
		return nil, errors.New("password was not hashed")
	}
	return &model.OwnerUser{ID: "ou-1", OwnerID: ownerID, FullName: fullName, Email: email, Status: "active"}, nil
}

func (s *stubOwnerRepo) GetOwnerUserByEmail(context.Context, string) (*model.OwnerUser, error) {
	return s.loginUser, s.loginErr
}

func (s *stubOwnerRepo) GetOwnerUserByID(context.Context, string) (*model.OwnerUser, error) {
	return s.userByID, s.userByIDErr
}

func (s *stubOwnerRepo) GetOwnerByID(context.Context, string) (*model.Owner, error) {
	return s.ownerByID, s.ownerErr
}

type stubTenantRepo struct {
	tenant *model.TenantAuth
	err    error
}

func (s *stubTenantRepo) GetByEmail(context.Context, string) (*model.TenantAuth, error) {
	return s.tenant, s.err
}
func (s *stubTenantRepo) GetByID(context.Context, string) (*model.TenantAuth, error) {
	return s.tenant, s.err
}

func testManager() *auth.Manager { return auth.NewManager("secret", 60, 720) }

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return string(h)
}

func TestRegisterOwner_HashesAndInserts(t *testing.T) {
	owners := &stubOwnerRepo{}
	svc := NewAuthService(owners, &stubTenantRepo{}, testManager())

	res, err := svc.RegisterOwner(context.Background(), model.RegisterOwnerInput{
		FullName: "Owner", Email: "o@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if res.Tokens.AccessToken == "" || res.Tokens.RefreshToken == "" {
		t.Fatal("expected tokens")
	}
	if len(owners.created) != 1 || owners.created[0] != "o@example.com" {
		t.Fatalf("owner user not created: %+v", owners.created)
	}
}

func TestRegisterOwner_EmailTaken(t *testing.T) {
	svc := NewAuthService(&stubOwnerRepo{emailTaken: true}, &stubTenantRepo{}, testManager())
	_, err := svc.RegisterOwner(context.Background(), model.RegisterOwnerInput{
		FullName: "Owner", Email: "o@example.com", Password: "password123",
	})
	if !errors.Is(err, repository.ErrEmailTaken) {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
}

func TestLoginOwner(t *testing.T) {
	active := &model.OwnerUser{ID: "ou-1", OwnerID: "owner-1", Email: "o@example.com", Status: "active", PasswordHash: mustHash(t, "password123")}

	tests := []struct {
		name    string
		repo    *stubOwnerRepo
		pass    string
		wantErr error
	}{
		{"success", &stubOwnerRepo{loginUser: active}, "password123", nil},
		{"wrong password", &stubOwnerRepo{loginUser: active}, "nope", ErrInvalidCredentials},
		{"unknown email", &stubOwnerRepo{loginErr: repository.ErrNotFound}, "password123", ErrInvalidCredentials},
		{"inactive", &stubOwnerRepo{loginUser: &model.OwnerUser{Status: "disabled", PasswordHash: mustHash(t, "password123")}}, "password123", ErrInactiveUser},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewAuthService(tc.repo, &stubTenantRepo{}, testManager())
			_, err := svc.LoginOwner(context.Background(), model.LoginInput{Email: "o@example.com", Password: tc.pass})
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLoginTenant_NullPasswordRejected(t *testing.T) {
	svc := NewAuthService(&stubOwnerRepo{}, &stubTenantRepo{
		tenant: &model.TenantAuth{ID: "t-1", OwnerID: "owner-1", PasswordHash: nil},
	}, testManager())
	_, err := svc.LoginTenant(context.Background(), model.LoginInput{Email: "t@example.com", Password: "x"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginTenant_Success(t *testing.T) {
	h := mustHash(t, "secret123")
	svc := NewAuthService(&stubOwnerRepo{}, &stubTenantRepo{
		tenant: &model.TenantAuth{ID: "t-1", OwnerID: "owner-1", FullName: "Tn", PasswordHash: &h},
	}, testManager())
	res, err := svc.LoginTenant(context.Background(), model.LoginInput{Email: "t@example.com", Password: "secret123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.TenantID != "t-1" || res.Tokens.AccessToken == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestGetOwnerProfile(t *testing.T) {
	biz := "Acme Kos"
	phone := "+628123"
	user := &model.OwnerUser{ID: "ou-1", OwnerID: "owner-1", FullName: "Iqbal", Email: "o@example.com", Status: "active", PasswordHash: mustHash(t, "password123")}
	owner := &model.Owner{ID: "owner-1", BusinessName: &biz, FullName: "Iqbal", Email: "o@example.com", PhoneNumber: &phone}

	t.Run("success", func(t *testing.T) {
		svc := NewAuthService(&stubOwnerRepo{userByID: user, ownerByID: owner}, &stubTenantRepo{}, testManager())
		p, err := svc.GetOwnerProfile(context.Background(), "owner-1", "ou-1")
		if err != nil {
			t.Fatalf("profile: %v", err)
		}
		if p.OwnerID != "owner-1" || p.OwnerUserID != "ou-1" || p.FullName != "Iqbal" || p.Status != "active" {
			t.Fatalf("unexpected profile: %+v", p)
		}
		if p.BusinessName == nil || *p.BusinessName != biz || p.PhoneNumber == nil || *p.PhoneNumber != phone {
			t.Fatalf("workspace fields missing: %+v", p)
		}
	})

	t.Run("owner mismatch is not found", func(t *testing.T) {
		other := &model.OwnerUser{ID: "ou-1", OwnerID: "owner-2", Status: "active"}
		svc := NewAuthService(&stubOwnerRepo{userByID: other, ownerByID: owner}, &stubTenantRepo{}, testManager())
		if _, err := svc.GetOwnerProfile(context.Background(), "owner-1", "ou-1"); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		svc := NewAuthService(&stubOwnerRepo{userByIDErr: repository.ErrNotFound}, &stubTenantRepo{}, testManager())
		if _, err := svc.GetOwnerProfile(context.Background(), "owner-1", "ou-1"); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})
}

func TestRefreshOwner(t *testing.T) {
	mgr := testManager()
	svc := NewAuthService(&stubOwnerRepo{}, &stubTenantRepo{}, mgr)
	_, refresh, _ := mgr.GenerateOwnerTokens("ou-1", "owner-1")

	tokens, err := svc.RefreshOwner(context.Background(), refresh)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("expected new access token")
	}

	// An access token must not be accepted as a refresh token.
	access, _, _ := mgr.GenerateOwnerTokens("ou-1", "owner-1")
	if _, err := svc.RefreshOwner(context.Background(), access); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

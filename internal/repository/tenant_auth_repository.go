package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// TenantAuthRepository defines the tenant lookups needed for authentication and
// the tenant portal profile. It is intentionally narrow; the full tenant
// repository lives with the tenant module.
type TenantAuthRepository interface {
	GetByEmail(ctx context.Context, email string) (*model.TenantAuth, error)
	GetByID(ctx context.Context, id string) (*model.TenantAuth, error)
}

type tenantAuthRepository struct {
	pool *pgxpool.Pool
}

// NewTenantAuthRepository returns a Postgres-backed TenantAuthRepository.
func NewTenantAuthRepository(pool *pgxpool.Pool) TenantAuthRepository {
	return &tenantAuthRepository{pool: pool}
}

const tenantAuthCols = `id, owner_id, full_name, email, phone_number, password_hash, status`

func scanTenantAuth(row pgx.Row) (*model.TenantAuth, error) {
	var t model.TenantAuth
	err := row.Scan(&t.ID, &t.OwnerID, &t.FullName, &t.Email, &t.PhoneNumber, &t.PasswordHash, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *tenantAuthRepository) GetByEmail(ctx context.Context, email string) (*model.TenantAuth, error) {
	q := `SELECT ` + tenantAuthCols + ` FROM tenants WHERE email = $1 AND deleted_at IS NULL`
	t, err := scanTenantAuth(r.pool.QueryRow(ctx, q, email))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get tenant by email: %w", err)
	}
	return t, err
}

func (r *tenantAuthRepository) GetByID(ctx context.Context, id string) (*model.TenantAuth, error) {
	q := `SELECT ` + tenantAuthCols + ` FROM tenants WHERE id = $1 AND deleted_at IS NULL`
	t, err := scanTenantAuth(r.pool.QueryRow(ctx, q, id))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get tenant by id: %w", err)
	}
	return t, err
}

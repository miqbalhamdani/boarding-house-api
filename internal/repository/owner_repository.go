package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// ErrEmailTaken is returned when an email already exists (unique violation).
var ErrEmailTaken = errors.New("email already taken")

// OwnerRepository defines persistence for owners and their login users.
type OwnerRepository interface {
	// CreateOwner inserts an owner row within the given transaction.
	CreateOwner(ctx context.Context, tx pgx.Tx, in model.RegisterOwnerInput) (*model.Owner, error)
	// CreateOwnerUser inserts an owner_users row within the given transaction.
	CreateOwnerUser(ctx context.Context, tx pgx.Tx, ownerID, fullName, email, passwordHash string) (*model.OwnerUser, error)
	// GetOwnerUserByEmail fetches an owner login user by email.
	GetOwnerUserByEmail(ctx context.Context, email string) (*model.OwnerUser, error)
	// GetOwnerByID fetches an owner workspace by its ID.
	GetOwnerByID(ctx context.Context, ownerID string) (*model.Owner, error)
	// GetOwnerUserByID fetches an owner login user by its ID.
	GetOwnerUserByID(ctx context.Context, ownerUserID string) (*model.OwnerUser, error)
	// BeginTx starts a transaction for the register flow.
	BeginTx(ctx context.Context) (pgx.Tx, error)
}

type ownerRepository struct {
	pool *pgxpool.Pool
}

// NewOwnerRepository returns a Postgres-backed OwnerRepository.
func NewOwnerRepository(pool *pgxpool.Pool) OwnerRepository {
	return &ownerRepository{pool: pool}
}

func (r *ownerRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

func (r *ownerRepository) CreateOwner(ctx context.Context, tx pgx.Tx, in model.RegisterOwnerInput) (*model.Owner, error) {
	const q = `
		INSERT INTO owners (business_name, full_name, email, phone_number)
		VALUES ($1, $2, $3, $4)
		RETURNING id, business_name, full_name, email, phone_number, created_at, updated_at`

	var o model.Owner
	err := tx.QueryRow(ctx, q,
		nullIfEmpty(in.BusinessName), in.FullName, in.Email, nullIfEmpty(in.PhoneNumber),
	).Scan(&o.ID, &o.BusinessName, &o.FullName, &o.Email, &o.PhoneNumber, &o.CreatedAt, &o.UpdatedAt)
	if isUniqueViolation(err) {
		return nil, ErrEmailTaken
	}
	if err != nil {
		return nil, fmt.Errorf("create owner: %w", err)
	}
	return &o, nil
}

func (r *ownerRepository) CreateOwnerUser(ctx context.Context, tx pgx.Tx, ownerID, fullName, email, passwordHash string) (*model.OwnerUser, error) {
	const q = `
		INSERT INTO owner_users (owner_id, full_name, email, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, owner_id, full_name, email, password_hash, status, created_at, updated_at`

	var u model.OwnerUser
	err := tx.QueryRow(ctx, q, ownerID, fullName, email, passwordHash).
		Scan(&u.ID, &u.OwnerID, &u.FullName, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if isUniqueViolation(err) {
		return nil, ErrEmailTaken
	}
	if err != nil {
		return nil, fmt.Errorf("create owner user: %w", err)
	}
	return &u, nil
}

func (r *ownerRepository) GetOwnerUserByEmail(ctx context.Context, email string) (*model.OwnerUser, error) {
	const q = `
		SELECT id, owner_id, full_name, email, password_hash, status, created_at, updated_at
		FROM owner_users
		WHERE email = $1 AND deleted_at IS NULL`

	var u model.OwnerUser
	err := r.pool.QueryRow(ctx, q, email).
		Scan(&u.ID, &u.OwnerID, &u.FullName, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get owner user by email: %w", err)
	}
	return &u, nil
}

func (r *ownerRepository) GetOwnerByID(ctx context.Context, ownerID string) (*model.Owner, error) {
	const q = `
		SELECT id, business_name, full_name, email, phone_number, created_at, updated_at
		FROM owners
		WHERE id = $1 AND deleted_at IS NULL`

	var o model.Owner
	err := r.pool.QueryRow(ctx, q, ownerID).
		Scan(&o.ID, &o.BusinessName, &o.FullName, &o.Email, &o.PhoneNumber, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get owner by id: %w", err)
	}
	return &o, nil
}

func (r *ownerRepository) GetOwnerUserByID(ctx context.Context, ownerUserID string) (*model.OwnerUser, error) {
	const q = `
		SELECT id, owner_id, full_name, email, password_hash, status, created_at, updated_at
		FROM owner_users
		WHERE id = $1 AND deleted_at IS NULL`

	var u model.OwnerUser
	err := r.pool.QueryRow(ctx, q, ownerUserID).
		Scan(&u.ID, &u.OwnerID, &u.FullName, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get owner user by id: %w", err)
	}
	return &u, nil
}

// nullIfEmpty returns nil for empty strings so optional columns store NULL.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isUniqueViolation reports whether err is a Postgres unique-constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

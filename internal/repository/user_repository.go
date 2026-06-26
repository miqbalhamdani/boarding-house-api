package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// UserRepository defines persistence operations for users.
type UserRepository interface {
	Create(ctx context.Context, in model.CreateUserInput) (*model.User, error)
	GetByID(ctx context.Context, id int64) (*model.User, error)
	List(ctx context.Context, limit, offset int) ([]model.User, error)
}

type userRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository returns a Postgres-backed UserRepository.
func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepository{pool: pool}
}

func (r *userRepository) Create(ctx context.Context, in model.CreateUserInput) (*model.User, error) {
	const q = `
		INSERT INTO users (name, email)
		VALUES ($1, $2)
		RETURNING id, name, email, created_at, updated_at`

	var u model.User
	err := r.pool.QueryRow(ctx, q, in.Name, in.Email).
		Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	const q = `
		SELECT id, name, email, created_at, updated_at
		FROM users
		WHERE id = $1`

	var u model.User
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

func (r *userRepository) List(ctx context.Context, limit, offset int) ([]model.User, error) {
	const q = `
		SELECT id, name, email, created_at, updated_at
		FROM users
		ORDER BY id
		LIMIT $1 OFFSET $2`

	rows, err := r.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.User, error) {
		var u model.User
		err := row.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
		return u, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan users: %w", err)
	}
	return users, nil
}

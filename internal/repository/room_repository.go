package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbal-hamdani/go-backend/internal/model"
)

// ErrRoomNumberTaken is returned when (owner_id, room_number) is not unique.
var ErrRoomNumberTaken = errors.New("room number already taken for this owner")

// RoomRepository defines persistence operations for rooms.
type RoomRepository interface {
	Create(ctx context.Context, ownerID string, in model.CreateRoomInput) (*model.Room, error)
	List(ctx context.Context, ownerID string, f model.ListRoomsFilter) (*model.ListRoomsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Room, error)
	// GetCurrentTenant returns the tenant on the room's active/pending
	// assignment, or ErrNotFound when the room is vacant.
	GetCurrentTenant(ctx context.Context, roomID, ownerID string) (*model.RoomCurrentTenant, error)
	Update(ctx context.Context, id, ownerID string, in model.UpdateRoomInput) (*model.Room, error)
	SoftDelete(ctx context.Context, id, ownerID string) error
}

type roomRepository struct {
	pool *pgxpool.Pool
}

// NewRoomRepository returns a Postgres-backed RoomRepository.
func NewRoomRepository(pool *pgxpool.Pool) RoomRepository {
	return &roomRepository{pool: pool}
}

func (r *roomRepository) Create(ctx context.Context, ownerID string, in model.CreateRoomInput) (*model.Room, error) {
	const q = `
		INSERT INTO rooms (owner_id, room_number, room_name, monthly_rent, status, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, owner_id, room_number, room_name, monthly_rent, status, notes, created_at, updated_at`

	var rm model.Room
	err := r.pool.QueryRow(ctx, q,
		ownerID,
		in.RoomNumber,
		nullIfEmpty(in.RoomName),
		in.MonthlyRent,
		in.Status,
		nullIfEmpty(in.Notes),
	).Scan(
		&rm.ID, &rm.OwnerID, &rm.RoomNumber, &rm.RoomName,
		&rm.MonthlyRent, &rm.Status, &rm.Notes,
		&rm.CreatedAt, &rm.UpdatedAt,
	)
	if isUniqueViolation(err) {
		return nil, ErrRoomNumberTaken
	}
	if err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	return &rm, nil
}

func (r *roomRepository) List(ctx context.Context, ownerID string, f model.ListRoomsFilter) (*model.ListRoomsResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	offset := (f.Page - 1) * f.Limit

	const q = `
		SELECT
			id, owner_id, room_number, room_name, monthly_rent, status, notes,
			created_at, updated_at,
			COUNT(*) OVER() AS total_count
		FROM rooms
		WHERE owner_id = $1
			AND deleted_at IS NULL
			AND ($2 = '' OR status = $2)
			AND ($3 = '' OR room_number ILIKE '%' || $3 || '%' OR room_name ILIKE '%' || $3 || '%')
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`

	rows, err := r.pool.Query(ctx, q, ownerID, f.Status, f.Search, f.Limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	defer rows.Close()

	var rooms []*model.Room
	var total int
	for rows.Next() {
		var rm model.Room
		if err := rows.Scan(
			&rm.ID, &rm.OwnerID, &rm.RoomNumber, &rm.RoomName,
			&rm.MonthlyRent, &rm.Status, &rm.Notes,
			&rm.CreatedAt, &rm.UpdatedAt,
			&total,
		); err != nil {
			return nil, fmt.Errorf("scan room: %w", err)
		}
		rooms = append(rooms, &rm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}

	if rooms == nil {
		rooms = []*model.Room{}
	}
	return &model.ListRoomsResult{
		Rooms: rooms,
		Total: total,
		Page:  f.Page,
		Limit: f.Limit,
	}, nil
}

func (r *roomRepository) GetByID(ctx context.Context, id, ownerID string) (*model.Room, error) {
	const q = `
		SELECT id, owner_id, room_number, room_name, monthly_rent, status, notes, created_at, updated_at
		FROM rooms
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	var rm model.Room
	err := r.pool.QueryRow(ctx, q, id, ownerID).Scan(
		&rm.ID, &rm.OwnerID, &rm.RoomNumber, &rm.RoomName,
		&rm.MonthlyRent, &rm.Status, &rm.Notes,
		&rm.CreatedAt, &rm.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get room by id: %w", err)
	}
	return &rm, nil
}

func (r *roomRepository) GetCurrentTenant(ctx context.Context, roomID, ownerID string) (*model.RoomCurrentTenant, error) {
	// The room's current tenant is whoever holds the active or pending
	// assignment; ended and cancelled assignments are historical. Newest wins.
	const q = `
		SELECT
			t.id, t.full_name, t.phone_number, t.email,
			ra.id, ra.status, ra.start_date, ra.end_date, ra.monthly_rent, ra.payment_due_day
		FROM room_assignments ra
		JOIN tenants t ON t.id = ra.tenant_id
		WHERE ra.room_id = $1 AND ra.owner_id = $2
			AND ra.status IN ('pending_payment', 'active')
			AND ra.deleted_at IS NULL
		ORDER BY ra.created_at DESC
		LIMIT 1`

	var v model.RoomCurrentTenant
	err := r.pool.QueryRow(ctx, q, roomID, ownerID).Scan(
		&v.TenantID, &v.FullName, &v.PhoneNumber, &v.Email,
		&v.RoomAssignmentID, &v.AssignmentStatus, &v.StartDate, &v.EndDate,
		&v.MonthlyRent, &v.PaymentDueDay,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get current tenant: %w", err)
	}
	return &v, nil
}

func (r *roomRepository) Update(ctx context.Context, id, ownerID string, in model.UpdateRoomInput) (*model.Room, error) {
	setClauses := []string{"updated_at = now()"}
	// $1 = id, $2 = owner_id are reserved for the WHERE clause.
	args := []any{id, ownerID}
	argIdx := 3

	if in.RoomNumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("room_number = $%d", argIdx))
		args = append(args, *in.RoomNumber)
		argIdx++
	}
	if in.RoomName != nil {
		setClauses = append(setClauses, fmt.Sprintf("room_name = $%d", argIdx))
		args = append(args, nullIfEmpty(*in.RoomName))
		argIdx++
	}
	if in.MonthlyRent != nil {
		setClauses = append(setClauses, fmt.Sprintf("monthly_rent = $%d", argIdx))
		args = append(args, *in.MonthlyRent)
		argIdx++
	}
	if in.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *in.Status)
		argIdx++
	}
	if in.Notes != nil {
		setClauses = append(setClauses, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, nullIfEmpty(*in.Notes))
		argIdx++
	}

	q := fmt.Sprintf(`
		UPDATE rooms
		SET %s
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		RETURNING id, owner_id, room_number, room_name, monthly_rent, status, notes, created_at, updated_at`,
		strings.Join(setClauses, ", "))

	var rm model.Room
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&rm.ID, &rm.OwnerID, &rm.RoomNumber, &rm.RoomName,
		&rm.MonthlyRent, &rm.Status, &rm.Notes,
		&rm.CreatedAt, &rm.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if isUniqueViolation(err) {
		return nil, ErrRoomNumberTaken
	}
	if err != nil {
		return nil, fmt.Errorf("update room: %w", err)
	}
	return &rm, nil
}

func (r *roomRepository) SoftDelete(ctx context.Context, id, ownerID string) error {
	const q = `
		UPDATE rooms
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, id, ownerID)
	if err != nil {
		return fmt.Errorf("soft delete room: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

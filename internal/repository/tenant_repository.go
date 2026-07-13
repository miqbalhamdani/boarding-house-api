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

// ErrTenantEmailTaken is returned when a tenant email collides with an existing
// account (tenant emails are globally unique to support tenant portal login).
var ErrTenantEmailTaken = errors.New("tenant email already in use")

// TenantRepository defines persistence operations for tenant profiles.
type TenantRepository interface {
	Create(ctx context.Context, ownerID string, in model.CreateTenantInput, passwordHash *string) (*model.Tenant, error)
	List(ctx context.Context, ownerID string, f model.ListTenantsFilter) (*model.ListTenantsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Tenant, error)
	// GetCurrentRoom returns the room on the tenant's active/pending
	// assignment, or ErrNotFound when the tenant has no current room.
	GetCurrentRoom(ctx context.Context, tenantID, ownerID string) (*model.TenantCurrentRoom, error)
	Update(ctx context.Context, id, ownerID string, in model.UpdateTenantInput, passwordHash *string) (*model.Tenant, error)
	SoftDelete(ctx context.Context, id, ownerID string) error
}

type tenantRepository struct {
	pool *pgxpool.Pool
}

// NewTenantRepository returns a Postgres-backed TenantRepository.
func NewTenantRepository(pool *pgxpool.Pool) TenantRepository {
	return &tenantRepository{pool: pool}
}

// tenantSelectCols is the projection used for reads. has_portal_access is derived
// from password_hash so the credential value itself is never selected.
const tenantSelectCols = `
	id, owner_id, full_name, phone_number, email, identity_number,
	emergency_contact_name, emergency_contact_phone, status,
	(password_hash IS NOT NULL) AS has_portal_access,
	created_at, updated_at`

func scanTenant(row pgx.Row) (*model.Tenant, error) {
	var t model.Tenant
	err := row.Scan(
		&t.ID, &t.OwnerID, &t.FullName, &t.PhoneNumber, &t.Email, &t.IdentityNumber,
		&t.EmergencyContactName, &t.EmergencyContactPhone, &t.Status,
		&t.HasPortalAccess, &t.CreatedAt, &t.UpdatedAt,
	)
	return &t, err
}

func (r *tenantRepository) Create(ctx context.Context, ownerID string, in model.CreateTenantInput, passwordHash *string) (*model.Tenant, error) {
	const q = `
		INSERT INTO tenants (
			owner_id, full_name, phone_number, email, password_hash, identity_number,
			emergency_contact_name, emergency_contact_phone, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + tenantSelectCols

	t, err := scanTenant(r.pool.QueryRow(ctx, q,
		ownerID,
		in.FullName,
		nullIfEmpty(in.PhoneNumber),
		nullIfEmpty(in.Email),
		passwordHash,
		nullIfEmpty(in.IdentityNumber),
		nullIfEmpty(in.EmergencyContactName),
		nullIfEmpty(in.EmergencyContactPhone),
		"pending_payment",
	))
	if isUniqueViolation(err) {
		return nil, ErrTenantEmailTaken
	}
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return t, nil
}

func (r *tenantRepository) List(ctx context.Context, ownerID string, f model.ListTenantsFilter) (*model.ListTenantsResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	offset := (f.Page - 1) * f.Limit

	const q = `
		SELECT
			id, owner_id, full_name, phone_number, email, identity_number,
			emergency_contact_name, emergency_contact_phone, status,
			(password_hash IS NOT NULL) AS has_portal_access,
			created_at, updated_at,
			COUNT(*) OVER() AS total_count
		FROM tenants
		WHERE owner_id = $1
			AND deleted_at IS NULL
			AND ($2 = '' OR status = $2)
			AND ($3 = '' OR full_name ILIKE '%' || $3 || '%'
				OR email ILIKE '%' || $3 || '%'
				OR phone_number ILIKE '%' || $3 || '%')
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`

	rows, err := r.pool.Query(ctx, q, ownerID, f.Status, f.Search, f.Limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	tenants := []*model.Tenant{}
	var total int
	for rows.Next() {
		var t model.Tenant
		if err := rows.Scan(
			&t.ID, &t.OwnerID, &t.FullName, &t.PhoneNumber, &t.Email, &t.IdentityNumber,
			&t.EmergencyContactName, &t.EmergencyContactPhone, &t.Status,
			&t.HasPortalAccess, &t.CreatedAt, &t.UpdatedAt,
			&total,
		); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}

	return &model.ListTenantsResult{
		Tenants: tenants,
		Total:   total,
		Page:    f.Page,
		Limit:   f.Limit,
	}, nil
}

func (r *tenantRepository) GetByID(ctx context.Context, id, ownerID string) (*model.Tenant, error) {
	const q = `SELECT ` + tenantSelectCols + `
		FROM tenants
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	t, err := scanTenant(r.pool.QueryRow(ctx, q, id, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant by id: %w", err)
	}
	return t, nil
}

func (r *tenantRepository) GetCurrentRoom(ctx context.Context, tenantID, ownerID string) (*model.TenantCurrentRoom, error) {
	// The tenant's current room is whichever room holds the active or pending
	// assignment; ended and cancelled assignments are historical. Newest wins.
	const q = `
		SELECT
			rm.id, rm.room_number, rm.room_name,
			ra.id, ra.status, ra.start_date, ra.end_date, ra.monthly_rent, ra.payment_due_day
		FROM room_assignments ra
		JOIN rooms rm ON rm.id = ra.room_id
		WHERE ra.tenant_id = $1 AND ra.owner_id = $2
			AND ra.status IN ('pending_payment', 'active')
			AND ra.deleted_at IS NULL
		ORDER BY ra.created_at DESC
		LIMIT 1`

	var v model.TenantCurrentRoom
	err := r.pool.QueryRow(ctx, q, tenantID, ownerID).Scan(
		&v.RoomID, &v.RoomNumber, &v.RoomName,
		&v.RoomAssignmentID, &v.AssignmentStatus, &v.StartDate, &v.EndDate,
		&v.MonthlyRent, &v.PaymentDueDay,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get current room: %w", err)
	}
	return &v, nil
}

func (r *tenantRepository) Update(ctx context.Context, id, ownerID string, in model.UpdateTenantInput, passwordHash *string) (*model.Tenant, error) {
	setClauses := []string{"updated_at = now()"}
	// $1 = id, $2 = owner_id are reserved for the WHERE clause.
	args := []any{id, ownerID}
	argIdx := 3

	add := func(col string, val any) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	if in.FullName != nil {
		add("full_name", *in.FullName)
	}
	if in.PhoneNumber != nil {
		add("phone_number", nullIfEmpty(*in.PhoneNumber))
	}
	if in.Email != nil {
		add("email", nullIfEmpty(*in.Email))
	}
	if in.IdentityNumber != nil {
		add("identity_number", nullIfEmpty(*in.IdentityNumber))
	}
	if in.EmergencyContactName != nil {
		add("emergency_contact_name", nullIfEmpty(*in.EmergencyContactName))
	}
	if in.EmergencyContactPhone != nil {
		add("emergency_contact_phone", nullIfEmpty(*in.EmergencyContactPhone))
	}
	if in.Status != nil {
		add("status", *in.Status)
	}
	if passwordHash != nil {
		add("password_hash", *passwordHash)
	}

	q := fmt.Sprintf(`
		UPDATE tenants
		SET %s
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		RETURNING `+tenantSelectCols,
		strings.Join(setClauses, ", "))

	t, err := scanTenant(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if isUniqueViolation(err) {
		return nil, ErrTenantEmailTaken
	}
	if err != nil {
		return nil, fmt.Errorf("update tenant: %w", err)
	}
	return t, nil
}

func (r *tenantRepository) SoftDelete(ctx context.Context, id, ownerID string) error {
	const q = `
		UPDATE tenants
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, id, ownerID)
	if err != nil {
		return fmt.Errorf("soft delete tenant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

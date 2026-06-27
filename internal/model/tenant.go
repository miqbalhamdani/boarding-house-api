package model

import "time"

// Tenant is the domain entity for a tenant profile owned by an owner workspace.
// Sensitive credential fields (password_hash) are never serialized to clients.
type Tenant struct {
	ID                    string    `json:"id"`
	OwnerID               string    `json:"owner_id"`
	FullName              string    `json:"full_name"`
	PhoneNumber           *string   `json:"phone_number,omitempty"`
	Email                 *string   `json:"email,omitempty"`
	IdentityNumber        *string   `json:"identity_number,omitempty"`
	EmergencyContactName  *string   `json:"emergency_contact_name,omitempty"`
	EmergencyContactPhone *string   `json:"emergency_contact_phone,omitempty"`
	Status                string    `json:"status"`
	HasPortalAccess       bool      `json:"has_portal_access"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// CreateTenantInput is the body for POST /owner/tenants.
// Status is always derived (defaults to pending_payment) and never accepted here.
// Password is optional: when provided, tenant portal credentials are generated.
type CreateTenantInput struct {
	FullName              string `json:"full_name"               binding:"required"`
	PhoneNumber           string `json:"phone_number"`
	Email                 string `json:"email"                   binding:"omitempty,email"`
	IdentityNumber        string `json:"identity_number"`
	EmergencyContactName  string `json:"emergency_contact_name"`
	EmergencyContactPhone string `json:"emergency_contact_phone"`
	Password              string `json:"password"                binding:"omitempty,min=6"`
}

// UpdateTenantInput is the body for PATCH /owner/tenants/{id}. All fields optional.
// Status may only be moved to moved_out or cancelled here; activation to "active"
// is governed by first-bill payment (BR-008) and is not a manual operation.
type UpdateTenantInput struct {
	FullName              *string `json:"full_name"               binding:"omitempty,min=1"`
	PhoneNumber           *string `json:"phone_number"`
	Email                 *string `json:"email"                   binding:"omitempty,email"`
	IdentityNumber        *string `json:"identity_number"`
	EmergencyContactName  *string `json:"emergency_contact_name"`
	EmergencyContactPhone *string `json:"emergency_contact_phone"`
	Status                *string `json:"status"                  binding:"omitempty,oneof=moved_out cancelled"`
	Password              *string `json:"password"                binding:"omitempty,min=6"`
}

// ListTenantsFilter carries query params for GET /owner/tenants.
type ListTenantsFilter struct {
	Status string
	Search string
	Page   int
	Limit  int
}

// ListTenantsResult is the paginated list response.
type ListTenantsResult struct {
	Tenants []*Tenant `json:"tenants"`
	Total   int       `json:"total"`
	Page    int       `json:"page"`
	Limit   int       `json:"limit"`
}

package model

import "time"

// Owner is a rental workspace/account.
type Owner struct {
	ID           string     `json:"id"`
	BusinessName *string    `json:"business_name,omitempty"`
	FullName     string     `json:"full_name"`
	Email        string     `json:"email"`
	PhoneNumber  *string    `json:"phone_number,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"-"`
}

// OwnerUser is a login user belonging to an owner workspace.
type OwnerUser struct {
	ID           string    `json:"id"`
	OwnerID      string    `json:"owner_id"`
	FullName     string    `json:"full_name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TenantAuth holds the tenant fields needed for authentication and /tenant/me.
type TenantAuth struct {
	ID           string  `json:"id"`
	OwnerID      string  `json:"owner_id"`
	FullName     string  `json:"full_name"`
	Email        *string `json:"email,omitempty"`
	PhoneNumber  *string `json:"phone_number,omitempty"`
	PasswordHash *string `json:"-"`
	Status       string  `json:"status"`
}

// --- Request DTOs ---

// RegisterOwnerInput is the body for POST /auth/owner/register.
type RegisterOwnerInput struct {
	BusinessName string `json:"business_name"`
	FullName     string `json:"full_name" binding:"required"`
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=8"`
	PhoneNumber  string `json:"phone_number"`
}

// LoginInput is the body for owner and tenant login.
type LoginInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshInput is the body for the refresh endpoints.
type RefreshInput struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// --- Response DTOs ---

// AuthTokens is the token pair returned by login/register/refresh.
type AuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// OwnerAuthResult is returned by register/login: profile plus tokens.
type OwnerAuthResult struct {
	OwnerID     string     `json:"owner_id"`
	OwnerUserID string     `json:"owner_user_id"`
	FullName    string     `json:"full_name"`
	Email       string     `json:"email"`
	Tokens      AuthTokens `json:"tokens"`
}

// TenantAuthResult is returned by tenant login: profile plus tokens.
type TenantAuthResult struct {
	TenantID string     `json:"tenant_id"`
	OwnerID  string     `json:"owner_id"`
	FullName string     `json:"full_name"`
	Tokens   AuthTokens `json:"tokens"`
}

// Package auth provides JWT generation and verification for owner and tenant
// access/refresh tokens. Tokens are stateless and signed with HMAC-SHA256.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token types carried in the "typ" claim so an access token cannot be used
// where a refresh token is expected (and vice versa).
const (
	TypeAccess  = "access"
	TypeRefresh = "refresh"
)

// ErrInvalidToken is returned for any malformed, expired, mis-signed, or
// wrong-type token.
var ErrInvalidToken = errors.New("invalid token")

// OwnerClaims are embedded in owner tokens.
type OwnerClaims struct {
	OwnerUserID string `json:"owner_user_id"`
	OwnerID     string `json:"owner_id"`
	Type        string `json:"typ"`
	jwt.RegisteredClaims
}

// TenantClaims are embedded in tenant tokens.
type TenantClaims struct {
	TenantID string `json:"tenant_id"`
	OwnerID  string `json:"owner_id"`
	Type     string `json:"typ"`
	jwt.RegisteredClaims
}

// Manager issues and verifies tokens using a shared secret and TTLs.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewManager builds a Manager. accessTTLMinutes and refreshTTLHours come from config.
func NewManager(secret string, accessTTLMinutes, refreshTTLHours int) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  time.Duration(accessTTLMinutes) * time.Minute,
		refreshTTL: time.Duration(refreshTTLHours) * time.Hour,
	}
}

func (m *Manager) sign(claims jwt.Claims) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return s, nil
}

func registered(ttl time.Duration) jwt.RegisteredClaims {
	now := time.Now()
	return jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
}

// GenerateOwnerTokens returns a signed access and refresh token for an owner user.
func (m *Manager) GenerateOwnerTokens(ownerUserID, ownerID string) (access, refresh string, err error) {
	access, err = m.sign(OwnerClaims{ownerUserID, ownerID, TypeAccess, registered(m.accessTTL)})
	if err != nil {
		return "", "", err
	}
	refresh, err = m.sign(OwnerClaims{ownerUserID, ownerID, TypeRefresh, registered(m.refreshTTL)})
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// GenerateTenantTokens returns a signed access and refresh token for a tenant.
func (m *Manager) GenerateTenantTokens(tenantID, ownerID string) (access, refresh string, err error) {
	access, err = m.sign(TenantClaims{tenantID, ownerID, TypeAccess, registered(m.accessTTL)})
	if err != nil {
		return "", "", err
	}
	refresh, err = m.sign(TenantClaims{tenantID, ownerID, TypeRefresh, registered(m.refreshTTL)})
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func (m *Manager) keyFunc(t *jwt.Token) (any, error) {
	if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, ErrInvalidToken
	}
	return m.secret, nil
}

// ParseOwner verifies an owner token and checks it carries the expected type.
func (m *Manager) ParseOwner(token, expectedType string) (*OwnerClaims, error) {
	var claims OwnerClaims
	if _, err := jwt.ParseWithClaims(token, &claims, m.keyFunc); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.Type != expectedType || claims.OwnerID == "" || claims.OwnerUserID == "" {
		return nil, ErrInvalidToken
	}
	return &claims, nil
}

// ParseTenant verifies a tenant token and checks it carries the expected type.
func (m *Manager) ParseTenant(token, expectedType string) (*TenantClaims, error) {
	var claims TenantClaims
	if _, err := jwt.ParseWithClaims(token, &claims, m.keyFunc); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.Type != expectedType || claims.OwnerID == "" || claims.TenantID == "" {
		return nil, ErrInvalidToken
	}
	return &claims, nil
}

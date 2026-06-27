package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/pkg/auth"
	"github.com/iqbal-hamdani/go-backend/pkg/response"
)

// Context keys for identity derived from a verified token. Handlers and services
// must read tenancy from these — never from the request body.
const (
	ctxOwnerID     = "owner_id"
	ctxOwnerUserID = "owner_user_id"
	ctxTenantID    = "tenant_id"
	ctxTenantOwner = "tenant_owner_id"
)

func bearerToken(c *gin.Context) (string, bool) {
	h := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

// RequireOwner verifies an owner access token and stores owner identity in context.
func RequireOwner(m *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "missing bearer token", nil)
			c.Abort()
			return
		}
		claims, err := m.ParseOwner(token, auth.TypeAccess)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid or expired token", nil)
			c.Abort()
			return
		}
		c.Set(ctxOwnerID, claims.OwnerID)
		c.Set(ctxOwnerUserID, claims.OwnerUserID)
		c.Next()
	}
}

// RequireTenant verifies a tenant access token and stores tenant identity in context.
func RequireTenant(m *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "missing bearer token", nil)
			c.Abort()
			return
		}
		claims, err := m.ParseTenant(token, auth.TypeAccess)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid or expired token", nil)
			c.Abort()
			return
		}
		c.Set(ctxTenantID, claims.TenantID)
		c.Set(ctxTenantOwner, claims.OwnerID)
		c.Next()
	}
}

// OwnerIDFromContext returns the authenticated owner ID. This is the owner data
// isolation helper: every owner-owned query must filter by this value.
func OwnerIDFromContext(c *gin.Context) string {
	return c.GetString(ctxOwnerID)
}

// OwnerUserIDFromContext returns the authenticated owner user ID.
func OwnerUserIDFromContext(c *gin.Context) string {
	return c.GetString(ctxOwnerUserID)
}

// TenantIDFromContext returns the authenticated tenant ID.
func TenantIDFromContext(c *gin.Context) string {
	return c.GetString(ctxTenantID)
}

// TenantOwnerIDFromContext returns the owner ID that the authenticated tenant belongs to.
func TenantOwnerIDFromContext(c *gin.Context) string {
	return c.GetString(ctxTenantOwner)
}

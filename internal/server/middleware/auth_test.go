package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/pkg/auth"
)

func setup() (*auth.Manager, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	mgr := auth.NewManager("test-secret", 60, 720)
	r := gin.New()
	r.GET("/owner", RequireOwner(mgr), func(c *gin.Context) {
		c.String(http.StatusOK, OwnerIDFromContext(c))
	})
	r.GET("/tenant", RequireTenant(mgr), func(c *gin.Context) {
		c.String(http.StatusOK, TenantIDFromContext(c))
	})
	return mgr, r
}

func do(r *gin.Engine, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireOwner_MissingToken(t *testing.T) {
	_, r := setup()
	if w := do(r, "/owner", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestRequireOwner_ValidToken(t *testing.T) {
	mgr, r := setup()
	access, _, _ := mgr.GenerateOwnerTokens("ou-1", "owner-1")
	w := do(r, "/owner", "Bearer "+access)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if w.Body.String() != "owner-1" {
		t.Fatalf("owner_id not in context, got %q", w.Body.String())
	}
}

func TestCrossTokenRejected(t *testing.T) {
	mgr, r := setup()
	ownerAccess, _, _ := mgr.GenerateOwnerTokens("ou-1", "owner-1")
	tenantAccess, _, _ := mgr.GenerateTenantTokens("t-1", "owner-1")

	// Tenant token on an owner route → 401.
	if w := do(r, "/owner", "Bearer "+tenantAccess); w.Code != http.StatusUnauthorized {
		t.Fatalf("tenant token on owner route: want 401, got %d", w.Code)
	}
	// Owner token on a tenant route → 401.
	if w := do(r, "/tenant", "Bearer "+ownerAccess); w.Code != http.StatusUnauthorized {
		t.Fatalf("owner token on tenant route: want 401, got %d", w.Code)
	}
}

func TestRefreshTokenRejectedOnGuard(t *testing.T) {
	mgr, r := setup()
	_, refresh, _ := mgr.GenerateOwnerTokens("ou-1", "owner-1")
	if w := do(r, "/owner", "Bearer "+refresh); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token accepted as access: got %d", w.Code)
	}
}

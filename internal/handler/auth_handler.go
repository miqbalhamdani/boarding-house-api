package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/internal/server/middleware"
	"github.com/iqbal-hamdani/go-backend/internal/service"
	"github.com/iqbal-hamdani/go-backend/pkg/auth"
	"github.com/iqbal-hamdani/go-backend/pkg/response"
)

// AuthHandler exposes authentication endpoints for owners and tenants.
type AuthHandler struct {
	svc service.AuthService
	mgr *auth.Manager
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(svc service.AuthService, mgr *auth.Manager) *AuthHandler {
	return &AuthHandler{svc: svc, mgr: mgr}
}

// Register attaches auth and tenant-portal routes to the given router group.
func (h *AuthHandler) Register(rg *gin.RouterGroup) {
	a := rg.Group("/auth")
	a.POST("/owner/register", h.RegisterOwner)
	a.POST("/owner/login", h.LoginOwner)
	a.POST("/owner/refresh", h.RefreshOwner)
	a.POST("/tenant/login", h.LoginTenant)
	a.POST("/tenant/refresh", h.RefreshTenant)

	t := rg.Group("/tenant")
	t.Use(middleware.RequireTenant(h.mgr))
	t.GET("/me", h.TenantMe)

	o := rg.Group("/owner")
	o.Use(middleware.RequireOwner(h.mgr))
	o.GET("/me", h.OwnerMe)
}

func (h *AuthHandler) RegisterOwner(c *gin.Context) {
	var in model.RegisterOwnerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	res, err := h.svc.RegisterOwner(c.Request.Context(), in)
	if errors.Is(err, repository.ErrEmailTaken) {
		response.Error(c, http.StatusConflict, response.CodeConflict, "email already registered", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not register owner", nil)
		return
	}
	response.Success(c, http.StatusCreated, res, "Owner registered")
}

func (h *AuthHandler) LoginOwner(c *gin.Context) {
	var in model.LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	res, err := h.svc.LoginOwner(c.Request.Context(), in)
	if errors.Is(err, service.ErrInvalidCredentials) {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid email or password", nil)
		return
	}
	if errors.Is(err, service.ErrInactiveUser) {
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "account is not active", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not log in", nil)
		return
	}
	response.Success(c, http.StatusOK, res, "Logged in")
}

func (h *AuthHandler) LoginTenant(c *gin.Context) {
	var in model.LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	res, err := h.svc.LoginTenant(c.Request.Context(), in)
	if errors.Is(err, service.ErrInvalidCredentials) {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid email or password", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not log in", nil)
		return
	}
	response.Success(c, http.StatusOK, res, "Logged in")
}

func (h *AuthHandler) RefreshOwner(c *gin.Context) {
	var in model.RefreshInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}
	tokens, err := h.svc.RefreshOwner(c.Request.Context(), in.RefreshToken)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid refresh token", nil)
		return
	}
	response.Success(c, http.StatusOK, tokens, "Token refreshed")
}

func (h *AuthHandler) RefreshTenant(c *gin.Context) {
	var in model.RefreshInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}
	tokens, err := h.svc.RefreshTenant(c.Request.Context(), in.RefreshToken)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid refresh token", nil)
		return
	}
	response.Success(c, http.StatusOK, tokens, "Token refreshed")
}

func (h *AuthHandler) TenantMe(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	profile, err := h.svc.GetTenantProfile(c.Request.Context(), tenantID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "tenant not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch profile", nil)
		return
	}
	response.Success(c, http.StatusOK, profile, "Success")
}

// OwnerMe returns the authenticated owner's profile. Identity is taken solely from
// the verified access token (context), never from the query string or body.
func (h *AuthHandler) OwnerMe(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	ownerUserID := middleware.OwnerUserIDFromContext(c)
	profile, err := h.svc.GetOwnerProfile(c.Request.Context(), ownerID, ownerUserID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "owner not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch profile", nil)
		return
	}
	response.Success(c, http.StatusOK, profile, "Success")
}

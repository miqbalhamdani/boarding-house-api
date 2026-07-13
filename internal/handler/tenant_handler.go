package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/internal/server/middleware"
	"github.com/iqbal-hamdani/go-backend/internal/service"
	"github.com/iqbal-hamdani/go-backend/pkg/auth"
	"github.com/iqbal-hamdani/go-backend/pkg/response"
)

// TenantHandler exposes tenant management endpoints for authenticated owners.
type TenantHandler struct {
	svc service.TenantService
	mgr *auth.Manager
}

// NewTenantHandler constructs a TenantHandler.
func NewTenantHandler(svc service.TenantService, mgr *auth.Manager) *TenantHandler {
	return &TenantHandler{svc: svc, mgr: mgr}
}

// Register attaches owner tenant routes to the given router group.
func (h *TenantHandler) Register(rg *gin.RouterGroup) {
	owner := rg.Group("/owner")
	owner.Use(middleware.RequireOwner(h.mgr))

	owner.GET("/tenants", h.ListTenants)
	owner.POST("/tenants", h.CreateTenant)
	owner.GET("/tenants/:tenant_id", h.GetTenant)
	owner.PATCH("/tenants/:tenant_id", h.UpdateTenant)
	owner.DELETE("/tenants/:tenant_id", h.DeleteTenant)
}

func (h *TenantHandler) ListTenants(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := model.ListTenantsFilter{
		Status: c.Query("status"),
		Search: c.Query("search"),
		Page:   page,
		Limit:  limit,
	}

	result, err := h.svc.List(c.Request.Context(), ownerID, filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list tenants", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Success")
}

func (h *TenantHandler) CreateTenant(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	var in model.CreateTenantInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	tenant, err := h.svc.Create(c.Request.Context(), ownerID, in)
	if errors.Is(err, repository.ErrTenantEmailTaken) {
		response.Error(c, http.StatusConflict, response.CodeConflict, "tenant email already in use", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not create tenant", nil)
		return
	}
	response.Success(c, http.StatusCreated, tenant, "Tenant created")
}

func (h *TenantHandler) GetTenant(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	tenantID := c.Param("tenant_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	billFilter := model.ListBillsFilter{
		Status: c.Query("status"),
		Page:   page,
		Limit:  limit,
	}

	detail, err := h.svc.GetDetail(c.Request.Context(), tenantID, ownerID, billFilter)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "tenant not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch tenant", nil)
		return
	}
	response.Success(c, http.StatusOK, detail, "Success")
}

func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	tenantID := c.Param("tenant_id")

	var in model.UpdateTenantInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	tenant, err := h.svc.Update(c.Request.Context(), tenantID, ownerID, in)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "tenant not found", nil)
		return
	}
	if errors.Is(err, repository.ErrTenantEmailTaken) {
		response.Error(c, http.StatusConflict, response.CodeConflict, "tenant email already in use", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not update tenant", nil)
		return
	}
	response.Success(c, http.StatusOK, tenant, "Tenant updated")
}

func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	tenantID := c.Param("tenant_id")

	err := h.svc.Delete(c.Request.Context(), tenantID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "tenant not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not delete tenant", nil)
		return
	}
	response.Success(c, http.StatusOK, nil, "Tenant deleted")
}

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

// TenantPortalHandler exposes the tenant portal endpoints. Every route is
// guarded by RequireTenant and derives tenant_id/owner_id from the token, never
// from the request (BR-002).
type TenantPortalHandler struct {
	svc service.TenantPortalService
	mgr *auth.Manager
}

// NewTenantPortalHandler constructs a TenantPortalHandler.
func NewTenantPortalHandler(svc service.TenantPortalService, mgr *auth.Manager) *TenantPortalHandler {
	return &TenantPortalHandler{svc: svc, mgr: mgr}
}

// Register attaches tenant portal routes to the given router group. The
// /tenant/me profile route lives with the auth module; this handler owns the
// portal data routes.
func (h *TenantPortalHandler) Register(rg *gin.RouterGroup) {
	t := rg.Group("/tenant")
	t.Use(middleware.RequireTenant(h.mgr))

	t.GET("/my-room", h.MyRoom)
	t.GET("/bills", h.ListBills)
	t.GET("/bills/:bill_id", h.GetBill)
	t.POST("/bills/:bill_id/pay", h.PayBill)
	t.GET("/payments", h.ListPayments)
}

func (h *TenantPortalHandler) MyRoom(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)

	room, err := h.svc.MyRoom(c.Request.Context(), tenantID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "no active room assignment", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch room", nil)
		return
	}
	response.Success(c, http.StatusOK, room, "Success")
}

func (h *TenantPortalHandler) ListBills(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := model.TenantListBillsFilter{
		Status: c.Query("status"),
		Page:   page,
		Limit:  limit,
	}

	result, err := h.svc.ListBills(c.Request.Context(), tenantID, ownerID, filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list bills", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Success")
}

func (h *TenantPortalHandler) GetBill(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)
	billID := c.Param("bill_id")

	bill, err := h.svc.GetBill(c.Request.Context(), billID, tenantID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "bill not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch bill", nil)
		return
	}
	response.Success(c, http.StatusOK, bill, "Success")
}

func (h *TenantPortalHandler) PayBill(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)
	billID := c.Param("bill_id")

	// Body is optional; provider defaults to the configured gateway provider.
	var in model.PayBillInput
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&in); err != nil {
			response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
			return
		}
	}

	result, err := h.svc.PayBill(c.Request.Context(), billID, tenantID, ownerID, in)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "bill not found", nil)
		return
	case errors.Is(err, service.ErrUnsupportedProvider):
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	case errors.Is(err, service.ErrBillAlreadyPaid),
		errors.Is(err, service.ErrBillNotPayable),
		errors.Is(err, repository.ErrDuplicateGatewayTransaction):
		response.Error(c, http.StatusConflict, response.CodeConflict, err.Error(), nil)
		return
	case err != nil:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not create checkout", nil)
		return
	}
	response.Success(c, http.StatusCreated, result, "Checkout created")
}

func (h *TenantPortalHandler) ListPayments(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := model.TenantListPaymentsFilter{
		Page:  page,
		Limit: limit,
	}

	result, err := h.svc.ListPayments(c.Request.Context(), tenantID, ownerID, filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list payments", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Success")
}

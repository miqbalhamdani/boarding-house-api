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

// BillHandler exposes monthly billing endpoints for authenticated owners.
type BillHandler struct {
	svc service.BillService
	mgr *auth.Manager
}

// NewBillHandler constructs a BillHandler.
func NewBillHandler(svc service.BillService, mgr *auth.Manager) *BillHandler {
	return &BillHandler{svc: svc, mgr: mgr}
}

// Register attaches owner billing routes to the given router group.
func (h *BillHandler) Register(rg *gin.RouterGroup) {
	owner := rg.Group("/owner")
	owner.Use(middleware.RequireOwner(h.mgr))

	owner.GET("/bills", h.ListBills)
	owner.GET("/bills/:bill_id", h.GetBill)
	owner.POST("/bills/generate-monthly", h.GenerateMonthly)
	owner.POST("/bills/mark-overdue", h.MarkOverdue)
}

func (h *BillHandler) ListBills(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := model.ListBillsFilter{
		Status:        c.Query("status"),
		BillingMonth:  c.Query("billing_month"),
		TenantID:      c.Query("tenant_id"),
		RoomID:        c.Query("room_id"),
		Page:          page,
		Limit:         limit,
		SortByDueDate: true,
	}

	result, err := h.svc.List(c.Request.Context(), ownerID, filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list bills", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Success")
}

func (h *BillHandler) GetBill(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	billID := c.Param("bill_id")

	bill, err := h.svc.GetByID(c.Request.Context(), billID, ownerID)
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

func (h *BillHandler) GenerateMonthly(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	var in model.GenerateMonthlyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	result, err := h.svc.GenerateMonthly(c.Request.Context(), ownerID, in)
	if errors.Is(err, service.ErrInvalidBillingMonth) {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not generate monthly bills", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Monthly bills generated")
}

func (h *BillHandler) MarkOverdue(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	result, err := h.svc.MarkOverdue(c.Request.Context(), ownerID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not mark overdue bills", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Overdue bills updated")
}

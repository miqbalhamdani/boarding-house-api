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

// PaymentHandler exposes payment-tracking endpoints for authenticated owners.
type PaymentHandler struct {
	svc service.PaymentService
	mgr *auth.Manager
}

// NewPaymentHandler constructs a PaymentHandler.
func NewPaymentHandler(svc service.PaymentService, mgr *auth.Manager) *PaymentHandler {
	return &PaymentHandler{svc: svc, mgr: mgr}
}

// Register attaches owner payment routes to the given router group.
func (h *PaymentHandler) Register(rg *gin.RouterGroup) {
	owner := rg.Group("/owner")
	owner.Use(middleware.RequireOwner(h.mgr))

	owner.POST("/payments/manual", h.RecordManual)
	owner.GET("/payments", h.ListPayments)
	owner.GET("/payments/:payment_id", h.GetPayment)
}

func (h *PaymentHandler) RecordManual(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	var in model.RecordManualPaymentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	payment, err := h.svc.RecordManual(c.Request.Context(), ownerID, in)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "bill not found", nil)
		return
	case errors.Is(err, service.ErrInvalidPaymentMethod),
		errors.Is(err, service.ErrInvalidPaymentDate),
		errors.Is(err, service.ErrPaymentAmountMismatch):
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	case errors.Is(err, service.ErrBillAlreadyPaid),
		errors.Is(err, service.ErrBillNotPayable),
		errors.Is(err, service.ErrDuplicatePayment):
		response.Error(c, http.StatusConflict, response.CodeConflict, err.Error(), nil)
		return
	case err != nil:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not record payment", nil)
		return
	}
	response.Success(c, http.StatusCreated, payment, "Payment recorded")
}

func (h *PaymentHandler) ListPayments(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := model.ListPaymentsFilter{
		TenantID: c.Query("tenant_id"),
		Month:    c.Query("month"),
		Page:     page,
		Limit:    limit,
	}

	result, err := h.svc.List(c.Request.Context(), ownerID, filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list payments", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Success")
}

func (h *PaymentHandler) GetPayment(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	paymentID := c.Param("payment_id")

	payment, err := h.svc.GetByID(c.Request.Context(), paymentID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "payment not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch payment", nil)
		return
	}
	response.Success(c, http.StatusOK, payment, "Success")
}

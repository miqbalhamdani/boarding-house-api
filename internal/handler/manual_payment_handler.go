package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/internal/server/middleware"
	"github.com/iqbal-hamdani/go-backend/internal/service"
	"github.com/iqbal-hamdani/go-backend/internal/storage"
	"github.com/iqbal-hamdani/go-backend/pkg/auth"
	"github.com/iqbal-hamdani/go-backend/pkg/response"
)

// multipartSlack allows for the non-file form fields and multipart boundary
// overhead on top of the 5MB proof file when bounding the request body.
const multipartSlack = 1 << 20 // 1MB

// ManualPaymentHandler exposes the manual payment-proof endpoints for both the
// tenant portal and the owner review workspace (Module 10).
type ManualPaymentHandler struct {
	svc service.ManualPaymentService
	mgr *auth.Manager
}

// NewManualPaymentHandler constructs a ManualPaymentHandler.
func NewManualPaymentHandler(svc service.ManualPaymentService, mgr *auth.Manager) *ManualPaymentHandler {
	return &ManualPaymentHandler{svc: svc, mgr: mgr}
}

// Register attaches tenant and owner routes to the given router group.
func (h *ManualPaymentHandler) Register(rg *gin.RouterGroup) {
	tenant := rg.Group("/tenant")
	tenant.Use(middleware.RequireTenant(h.mgr))
	tenant.POST("/bills/:bill_id/manual-payment-submissions", h.Submit)
	tenant.GET("/bills/:bill_id/manual-payment-submission", h.GetForTenant)
	tenant.POST("/manual-payment-submissions/:submission_id/cancel", h.Cancel)

	owner := rg.Group("/owner")
	owner.Use(middleware.RequireOwner(h.mgr))
	owner.GET("/manual-payment-submissions", h.List)
	owner.GET("/manual-payment-submissions/:submission_id", h.GetDetail)
	owner.POST("/manual-payment-submissions/:submission_id/approve", h.Approve)
	owner.POST("/manual-payment-submissions/:submission_id/reject", h.Reject)
}

func (h *ManualPaymentHandler) Submit(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)
	billID := c.Param("bill_id")

	// Bound the request body before parsing (defense in depth on top of the
	// exact 5MB check inside storage.DetectAndValidate).
	maxBody := int64(storage.MaxProofFileSize + multipartSlack)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	if err := c.Request.ParseMultipartForm(maxBody); err != nil {
		response.Error(c, http.StatusRequestEntityTooLarge, response.CodeValidation, "request body too large", nil)
		return
	}

	fileHeader, err := c.FormFile("proof")
	if err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, "proof file is required", nil)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, "could not read proof file", nil)
		return
	}
	defer file.Close()
	// Read one byte past the limit so an oversized file is detectable even if the
	// multipart header under-reported its size.
	content, err := io.ReadAll(io.LimitReader(file, storage.MaxProofFileSize+1))
	if err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, "could not read proof file", nil)
		return
	}

	amount, _ := strconv.Atoi(c.PostForm("submitted_amount"))
	in := model.SubmitManualPaymentInput{
		SubmittedAmount:   amount,
		PaymentMethod:     c.PostForm("payment_method"),
		TransferDate:      c.PostForm("transfer_date"),
		SenderAccountName: c.PostForm("sender_account_name"),
		ReferenceNumber:   c.PostForm("reference_number"),
		Notes:             c.PostForm("notes"),
		ProofFileName:     fileHeader.Filename,
		ProofContent:      content,
	}

	sub, err := h.svc.Submit(c.Request.Context(), tenantID, ownerID, billID, in)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "bill not found", nil)
	case errors.Is(err, service.ErrBillNotSubmittable):
		response.Error(c, http.StatusConflict, response.CodeConflict, err.Error(), nil)
	case errors.Is(err, service.ErrDuplicatePendingSubmission):
		response.Error(c, http.StatusConflict, response.CodeConflict, err.Error(), nil)
	case errors.Is(err, service.ErrSubmissionAmountMismatch),
		errors.Is(err, service.ErrInvalidPaymentMethod),
		errors.Is(err, service.ErrInvalidPaymentDate),
		errors.Is(err, service.ErrProofRequired),
		errors.Is(err, storage.ErrUnsupportedContentType),
		errors.Is(err, storage.ErrFileTooLarge),
		errors.Is(err, storage.ErrEmptyFile):
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
	case err != nil:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not submit payment proof", nil)
	default:
		response.Success(c, http.StatusCreated, sub, "Payment proof submitted for review")
	}
}

func (h *ManualPaymentHandler) GetForTenant(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)
	billID := c.Param("bill_id")

	sub, err := h.svc.GetForTenant(c.Request.Context(), tenantID, ownerID, billID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "no submission found for this bill", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch submission", nil)
		return
	}
	response.Success(c, http.StatusOK, sub, "Success")
}

func (h *ManualPaymentHandler) Cancel(c *gin.Context) {
	tenantID := middleware.TenantIDFromContext(c)
	ownerID := middleware.TenantOwnerIDFromContext(c)
	submissionID := c.Param("submission_id")

	err := h.svc.Cancel(c.Request.Context(), tenantID, ownerID, submissionID)
	if errors.Is(err, repository.ErrNotFound) {
		// Covers not-found, not-owned, and not-pending uniformly.
		response.Error(c, http.StatusConflict, response.CodeConflict, "submission cannot be cancelled", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not cancel submission", nil)
		return
	}
	response.Success(c, http.StatusOK, nil, "Submission cancelled")
}

func (h *ManualPaymentHandler) List(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := model.ListManualPaymentSubmissionsFilter{
		Status:       c.Query("status"),
		TenantID:     c.Query("tenant_id"),
		BillingMonth: c.Query("billing_month"),
		Page:         page,
		Limit:        limit,
	}

	result, err := h.svc.List(c.Request.Context(), ownerID, filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list submissions", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Success")
}

func (h *ManualPaymentHandler) GetDetail(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	submissionID := c.Param("submission_id")

	sub, err := h.svc.GetDetailForOwner(c.Request.Context(), ownerID, submissionID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "submission not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch submission", nil)
		return
	}
	response.Success(c, http.StatusOK, sub, "Success")
}

func (h *ManualPaymentHandler) Approve(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	reviewerID := middleware.OwnerUserIDFromContext(c)
	submissionID := c.Param("submission_id")

	var in model.ReviewSubmissionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	payment, err := h.svc.Approve(c.Request.Context(), ownerID, submissionID, reviewerID, in)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "submission not found", nil)
	case errors.Is(err, service.ErrSubmissionNotPending):
		response.Error(c, http.StatusConflict, response.CodeConflict, err.Error(), nil)
	case errors.Is(err, service.ErrBillAlreadyPaid),
		errors.Is(err, service.ErrBillNotPayable),
		errors.Is(err, service.ErrDuplicatePayment):
		response.Error(c, http.StatusConflict, response.CodeConflict, err.Error(), nil)
	case errors.Is(err, service.ErrPaymentAmountMismatch):
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
	case err != nil:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not approve submission", nil)
	default:
		response.Success(c, http.StatusOK, payment, "Payment approved")
	}
}

func (h *ManualPaymentHandler) Reject(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	reviewerID := middleware.OwnerUserIDFromContext(c)
	submissionID := c.Param("submission_id")

	var in model.RejectSubmissionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	err := h.svc.Reject(c.Request.Context(), ownerID, submissionID, reviewerID, in)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "submission not found", nil)
	case errors.Is(err, service.ErrRejectReasonRequired):
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
	case errors.Is(err, service.ErrSubmissionNotPending):
		response.Error(c, http.StatusConflict, response.CodeConflict, err.Error(), nil)
	case err != nil:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not reject submission", nil)
	default:
		response.Success(c, http.StatusOK, nil, "Submission rejected")
	}
}

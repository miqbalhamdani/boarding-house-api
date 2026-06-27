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

// OnboardingHandler exposes tenant onboarding endpoints for authenticated owners.
type OnboardingHandler struct {
	svc service.OnboardingService
	mgr *auth.Manager
}

// NewOnboardingHandler constructs an OnboardingHandler.
func NewOnboardingHandler(svc service.OnboardingService, mgr *auth.Manager) *OnboardingHandler {
	return &OnboardingHandler{svc: svc, mgr: mgr}
}

// Register attaches owner onboarding routes to the given router group.
func (h *OnboardingHandler) Register(rg *gin.RouterGroup) {
	owner := rg.Group("/owner")
	owner.Use(middleware.RequireOwner(h.mgr))

	owner.POST("/onboarding/assign-room", h.AssignRoom)
	owner.POST("/onboarding/:room_assignment_id/cancel", h.Cancel)
}

func (h *OnboardingHandler) AssignRoom(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	var in model.AssignRoomInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	result, err := h.svc.AssignRoom(c.Request.Context(), ownerID, in)
	switch {
	case errors.Is(err, service.ErrInvalidStartDate):
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	case errors.Is(err, repository.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "tenant or room not found", nil)
		return
	case errors.Is(err, service.ErrRoomNotAvailable):
		response.Error(c, http.StatusConflict, response.CodeConflict, "room is not available for assignment", nil)
		return
	case errors.Is(err, service.ErrRoomHasActiveAssignment):
		response.Error(c, http.StatusConflict, response.CodeConflict, "room already has an active or pending assignment", nil)
		return
	case errors.Is(err, service.ErrTenantHasActiveAssignment):
		response.Error(c, http.StatusConflict, response.CodeConflict, "tenant already has an active or pending assignment", nil)
		return
	case errors.Is(err, repository.ErrDuplicateBill):
		response.Error(c, http.StatusConflict, response.CodeConflict, "first bill already exists for this assignment", nil)
		return
	case err != nil:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not assign tenant to room", nil)
		return
	}

	response.Success(c, http.StatusCreated, result, "Tenant assigned to room")
}

func (h *OnboardingHandler) Cancel(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	assignmentID := c.Param("room_assignment_id")

	err := h.svc.Cancel(c.Request.Context(), ownerID, assignmentID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "room assignment not found", nil)
		return
	case errors.Is(err, service.ErrOnboardingNotCancelable):
		response.Error(c, http.StatusConflict, response.CodeConflict, "only a pending_payment onboarding can be cancelled", nil)
		return
	case err != nil:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not cancel onboarding", nil)
		return
	}

	response.Success(c, http.StatusOK, nil, "Onboarding cancelled")
}

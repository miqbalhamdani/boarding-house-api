package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/internal/server/middleware"
	"github.com/iqbal-hamdani/go-backend/internal/service"
	"github.com/iqbal-hamdani/go-backend/pkg/auth"
	"github.com/iqbal-hamdani/go-backend/pkg/response"
)

// DashboardHandler exposes the owner dashboard summary endpoint (Module 07).
type DashboardHandler struct {
	svc service.DashboardService
	mgr *auth.Manager
}

// NewDashboardHandler constructs a DashboardHandler.
func NewDashboardHandler(svc service.DashboardService, mgr *auth.Manager) *DashboardHandler {
	return &DashboardHandler{svc: svc, mgr: mgr}
}

// Register attaches owner dashboard routes to the given router group.
func (h *DashboardHandler) Register(rg *gin.RouterGroup) {
	owner := rg.Group("/owner")
	owner.Use(middleware.RequireOwner(h.mgr))

	owner.GET("/dashboard/summary", h.Summary)
}

func (h *DashboardHandler) Summary(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	month := c.Query("month")

	view, err := h.svc.Overview(c.Request.Context(), ownerID, month)
	if errors.Is(err, service.ErrInvalidMonth) {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not load dashboard summary", nil)
		return
	}
	response.Success(c, http.StatusOK, view, "Success")
}

package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/internal/service"
	"github.com/iqbal-hamdani/go-backend/pkg/response"
)

// UserHandler exposes HTTP endpoints for users.
type UserHandler struct {
	svc service.UserService
}

// NewUserHandler constructs a UserHandler.
func NewUserHandler(svc service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// Register attaches user routes to the given router group.
func (h *UserHandler) Register(rg *gin.RouterGroup) {
	users := rg.Group("/users")
	users.POST("", h.Create)
	users.GET("", h.List)
	users.GET("/:id", h.GetByID)
}

func (h *UserHandler) Create(c *gin.Context) {
	var in model.CreateUserInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	user, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not create user", nil)
		return
	}
	response.Success(c, http.StatusCreated, user, "User created")
}

func (h *UserHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, "invalid id", nil)
		return
	}

	user, err := h.svc.GetByID(c.Request.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "user not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch user", nil)
		return
	}
	response.Success(c, http.StatusOK, user, "Success")
}

func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	users, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list users", nil)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"items": users, "page": page, "page_size": pageSize}, "Success")
}

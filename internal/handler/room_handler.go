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

// RoomHandler exposes room management endpoints for authenticated owners.
type RoomHandler struct {
	svc service.RoomService
	mgr *auth.Manager
}

// NewRoomHandler constructs a RoomHandler.
func NewRoomHandler(svc service.RoomService, mgr *auth.Manager) *RoomHandler {
	return &RoomHandler{svc: svc, mgr: mgr}
}

// Register attaches owner room routes to the given router group.
func (h *RoomHandler) Register(rg *gin.RouterGroup) {
	owner := rg.Group("/owner")
	owner.Use(middleware.RequireOwner(h.mgr))

	owner.GET("/rooms", h.ListRooms)
	owner.POST("/rooms", h.CreateRoom)
	owner.GET("/rooms/:room_id", h.GetRoom)
	owner.PATCH("/rooms/:room_id", h.UpdateRoom)
	owner.DELETE("/rooms/:room_id", h.DeleteRoom)
}

func (h *RoomHandler) ListRooms(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := model.ListRoomsFilter{
		Status: c.Query("status"),
		Search: c.Query("search"),
		Page:   page,
		Limit:  limit,
	}

	result, err := h.svc.List(c.Request.Context(), ownerID, filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not list rooms", nil)
		return
	}
	response.Success(c, http.StatusOK, result, "Success")
}

func (h *RoomHandler) CreateRoom(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)

	var in model.CreateRoomInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	room, err := h.svc.Create(c.Request.Context(), ownerID, in)
	if errors.Is(err, repository.ErrRoomNumberTaken) {
		response.Error(c, http.StatusConflict, response.CodeConflict, "room number already exists for this owner", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not create room", nil)
		return
	}
	response.Success(c, http.StatusCreated, room, "Room created")
}

func (h *RoomHandler) GetRoom(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	roomID := c.Param("room_id")

	room, err := h.svc.GetByID(c.Request.Context(), roomID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "room not found", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not fetch room", nil)
		return
	}
	response.Success(c, http.StatusOK, room, "Success")
}

func (h *RoomHandler) UpdateRoom(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	roomID := c.Param("room_id")

	var in model.UpdateRoomInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeValidation, err.Error(), nil)
		return
	}

	room, err := h.svc.Update(c.Request.Context(), roomID, ownerID, in)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "room not found", nil)
		return
	}
	if errors.Is(err, repository.ErrRoomNumberTaken) {
		response.Error(c, http.StatusConflict, response.CodeConflict, "room number already exists for this owner", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not update room", nil)
		return
	}
	response.Success(c, http.StatusOK, room, "Room updated")
}

func (h *RoomHandler) DeleteRoom(c *gin.Context) {
	ownerID := middleware.OwnerIDFromContext(c)
	roomID := c.Param("room_id")

	err := h.svc.Delete(c.Request.Context(), roomID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "room not found", nil)
		return
	}
	if errors.Is(err, service.ErrRoomNotDeletable) {
		response.Error(c, http.StatusConflict, response.CodeConflict, "room is occupied or reserved and cannot be deleted", nil)
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "could not delete room", nil)
		return
	}
	response.Success(c, http.StatusOK, nil, "Room deleted")
}

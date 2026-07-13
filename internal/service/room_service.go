package service

import (
	"context"
	"errors"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

// ErrRoomNotDeletable is returned when an owner tries to delete an occupied or
// reserved room. The room must first be vacated before it can be removed.
var ErrRoomNotDeletable = errors.New("room is occupied or reserved and cannot be deleted")

// RoomService holds room management business logic.
type RoomService interface {
	Create(ctx context.Context, ownerID string, in model.CreateRoomInput) (*model.Room, error)
	List(ctx context.Context, ownerID string, f model.ListRoomsFilter) (*model.ListRoomsResult, error)
	GetByID(ctx context.Context, id, ownerID string) (*model.Room, error)
	// GetDetail returns a room together with its current tenant (if any) and
	// paginated bill history.
	GetDetail(ctx context.Context, id, ownerID string, billFilter model.ListBillsFilter) (*model.RoomDetailView, error)
	Update(ctx context.Context, id, ownerID string, in model.UpdateRoomInput) (*model.Room, error)
	Delete(ctx context.Context, id, ownerID string) error
}

type roomService struct {
	repo     repository.RoomRepository
	billRepo repository.BillRepository
}

// NewRoomService wires a RoomService to its repositories.
func NewRoomService(repo repository.RoomRepository, billRepo repository.BillRepository) RoomService {
	return &roomService{repo: repo, billRepo: billRepo}
}

func (s *roomService) Create(ctx context.Context, ownerID string, in model.CreateRoomInput) (*model.Room, error) {
	if in.Status == "" {
		in.Status = "available"
	}
	return s.repo.Create(ctx, ownerID, in)
}

func (s *roomService) List(ctx context.Context, ownerID string, f model.ListRoomsFilter) (*model.ListRoomsResult, error) {
	return s.repo.List(ctx, ownerID, f)
}

func (s *roomService) GetByID(ctx context.Context, id, ownerID string) (*model.Room, error) {
	return s.repo.GetByID(ctx, id, ownerID)
}

func (s *roomService) GetDetail(ctx context.Context, id, ownerID string, billFilter model.ListBillsFilter) (*model.RoomDetailView, error) {
	room, err := s.repo.GetByID(ctx, id, ownerID)
	if err != nil {
		return nil, err
	}

	currentTenant, err := s.repo.GetCurrentTenant(ctx, id, ownerID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if errors.Is(err, repository.ErrNotFound) {
		currentTenant = nil
	}

	billFilter.RoomID = id
	billHistory, err := s.billRepo.List(ctx, ownerID, billFilter)
	if err != nil {
		return nil, err
	}

	return &model.RoomDetailView{Room: room, CurrentTenant: currentTenant, BillHistory: billHistory}, nil
}

func (s *roomService) Update(ctx context.Context, id, ownerID string, in model.UpdateRoomInput) (*model.Room, error) {
	return s.repo.Update(ctx, id, ownerID, in)
}

func (s *roomService) Delete(ctx context.Context, id, ownerID string) error {
	room, err := s.repo.GetByID(ctx, id, ownerID)
	if err != nil {
		return err
	}
	if room.Status == "occupied" || room.Status == "reserved" {
		return ErrRoomNotDeletable
	}
	return s.repo.SoftDelete(ctx, id, ownerID)
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/iqbal-hamdani/go-backend/internal/model"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
)

type stubRoomRepo struct {
	room      *model.Room
	createErr error
	getErr    error
	deleteErr error
	deletedID string

	currentTenant    *model.RoomCurrentTenant
	currentTenantErr error
}

func (s *stubRoomRepo) Create(_ context.Context, _ string, _ model.CreateRoomInput) (*model.Room, error) {
	return s.room, s.createErr
}

func (s *stubRoomRepo) List(_ context.Context, _ string, f model.ListRoomsFilter) (*model.ListRoomsResult, error) {
	rooms := []*model.Room{}
	total := 0
	if s.room != nil {
		rooms = []*model.Room{s.room}
		total = 1
	}
	return &model.ListRoomsResult{Rooms: rooms, Total: total, Page: f.Page, Limit: f.Limit}, nil
}

func (s *stubRoomRepo) GetByID(_ context.Context, id, _ string) (*model.Room, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.room != nil {
		s.room.ID = id
	}
	return s.room, nil
}

func (s *stubRoomRepo) GetCurrentTenant(_ context.Context, _, _ string) (*model.RoomCurrentTenant, error) {
	return s.currentTenant, s.currentTenantErr
}

func (s *stubRoomRepo) Update(_ context.Context, _ string, _ string, _ model.UpdateRoomInput) (*model.Room, error) {
	return s.room, s.createErr
}

func (s *stubRoomRepo) SoftDelete(_ context.Context, id, _ string) error {
	s.deletedID = id
	return s.deleteErr
}

func TestCreateRoom_DefaultsStatusToAvailable(t *testing.T) {
	room := &model.Room{ID: "r-1", OwnerID: "o-1", RoomNumber: "101", MonthlyRent: 500000, Status: "available"}
	svc := NewRoomService(&stubRoomRepo{room: room}, &stubBillRepo{})

	in := model.CreateRoomInput{RoomNumber: "101", MonthlyRent: 500000}
	got, err := svc.Create(context.Background(), "o-1", in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "available" {
		t.Fatalf("want status=available, got %q", got.Status)
	}
}

func TestCreateRoom_RoomNumberTaken(t *testing.T) {
	svc := NewRoomService(&stubRoomRepo{createErr: repository.ErrRoomNumberTaken}, &stubBillRepo{})

	_, err := svc.Create(context.Background(), "o-1", model.CreateRoomInput{RoomNumber: "101", MonthlyRent: 500000})
	if !errors.Is(err, repository.ErrRoomNumberTaken) {
		t.Fatalf("want ErrRoomNumberTaken, got %v", err)
	}
}

func TestGetRoom_CrossOwnerIsolated(t *testing.T) {
	// The repository filters by owner_id; cross-owner access surfaces as ErrNotFound.
	svc := NewRoomService(&stubRoomRepo{getErr: repository.ErrNotFound}, &stubBillRepo{})

	_, err := svc.GetByID(context.Background(), "r-other", "o-1")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-owner access, got %v", err)
	}
}

func TestDeleteRoom_OccupiedRejected(t *testing.T) {
	room := &model.Room{ID: "r-1", OwnerID: "o-1", Status: "occupied"}
	svc := NewRoomService(&stubRoomRepo{room: room}, &stubBillRepo{})

	if err := svc.Delete(context.Background(), "r-1", "o-1"); !errors.Is(err, ErrRoomNotDeletable) {
		t.Fatalf("want ErrRoomNotDeletable for occupied room, got %v", err)
	}
}

func TestDeleteRoom_ReservedRejected(t *testing.T) {
	room := &model.Room{ID: "r-1", OwnerID: "o-1", Status: "reserved"}
	svc := NewRoomService(&stubRoomRepo{room: room}, &stubBillRepo{})

	if err := svc.Delete(context.Background(), "r-1", "o-1"); !errors.Is(err, ErrRoomNotDeletable) {
		t.Fatalf("want ErrRoomNotDeletable for reserved room, got %v", err)
	}
}

func TestDeleteRoom_AvailableSucceeds(t *testing.T) {
	room := &model.Room{ID: "r-1", OwnerID: "o-1", Status: "available"}
	stub := &stubRoomRepo{room: room}
	svc := NewRoomService(stub, &stubBillRepo{})

	if err := svc.Delete(context.Background(), "r-1", "o-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.deletedID != "r-1" {
		t.Fatalf("expected soft delete to be called with r-1, got %q", stub.deletedID)
	}
}

func TestDeleteRoom_NotFound(t *testing.T) {
	svc := NewRoomService(&stubRoomRepo{getErr: repository.ErrNotFound}, &stubBillRepo{})

	if err := svc.Delete(context.Background(), "r-missing", "o-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListRooms_ReturnsOwnRoomsOnly(t *testing.T) {
	room := &model.Room{ID: "r-1", OwnerID: "o-1", RoomNumber: "101", Status: "available"}
	svc := NewRoomService(&stubRoomRepo{room: room}, &stubBillRepo{})

	result, err := svc.List(context.Background(), "o-1", model.ListRoomsFilter{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 || len(result.Rooms) != 1 {
		t.Fatalf("want 1 room, got total=%d rooms=%d", result.Total, len(result.Rooms))
	}
	// The stub always returns the same room regardless of ownerID; isolation is
	// enforced at the repository (DB query) level, tested by cross-owner test above.
	if result.Rooms[0].OwnerID != "o-1" {
		t.Fatalf("room owner_id mismatch: %q", result.Rooms[0].OwnerID)
	}
}

func TestGetRoomDetail_IncludesCurrentTenantAndBills(t *testing.T) {
	room := &model.Room{ID: "r-1", OwnerID: "o-1", RoomNumber: "101", Status: "occupied"}
	tenant := &model.RoomCurrentTenant{TenantID: "t-1", FullName: "Jane Doe", RoomAssignmentID: "ra-1", AssignmentStatus: "active"}
	bills := &model.ListBillsResult{Bills: []*model.Bill{{ID: "b-1", RoomID: "r-1"}}, Total: 1, Page: 1, Limit: 20}
	svc := NewRoomService(
		&stubRoomRepo{room: room, currentTenant: tenant},
		&stubBillRepo{listResult: bills},
	)

	detail, err := svc.GetDetail(context.Background(), "r-1", "o-1", model.ListBillsFilter{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Room.ID != "r-1" {
		t.Fatalf("want room r-1, got %q", detail.Room.ID)
	}
	if detail.CurrentTenant == nil || detail.CurrentTenant.TenantID != "t-1" {
		t.Fatalf("want current tenant t-1, got %+v", detail.CurrentTenant)
	}
	if detail.BillHistory.Total != 1 || len(detail.BillHistory.Bills) != 1 {
		t.Fatalf("want 1 bill, got total=%d bills=%d", detail.BillHistory.Total, len(detail.BillHistory.Bills))
	}
}

func TestGetRoomDetail_VacantRoomHasNilCurrentTenant(t *testing.T) {
	room := &model.Room{ID: "r-1", OwnerID: "o-1", RoomNumber: "101", Status: "available"}
	bills := &model.ListBillsResult{Bills: []*model.Bill{}, Total: 0, Page: 1, Limit: 20}
	svc := NewRoomService(
		&stubRoomRepo{room: room, currentTenantErr: repository.ErrNotFound},
		&stubBillRepo{listResult: bills},
	)

	detail, err := svc.GetDetail(context.Background(), "r-1", "o-1", model.ListBillsFilter{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.CurrentTenant != nil {
		t.Fatalf("want nil current tenant for vacant room, got %+v", detail.CurrentTenant)
	}
}

func TestGetRoomDetail_CrossOwnerIsolated(t *testing.T) {
	svc := NewRoomService(&stubRoomRepo{getErr: repository.ErrNotFound}, &stubBillRepo{})

	_, err := svc.GetDetail(context.Background(), "r-other", "o-1", model.ListBillsFilter{Page: 1, Limit: 20})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-owner access, got %v", err)
	}
}

package model

import "time"

// Room is the domain entity for a rental room.
type Room struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"owner_id"`
	RoomNumber  string     `json:"room_number"`
	RoomName    *string    `json:"room_name,omitempty"`
	MonthlyRent int        `json:"monthly_rent"`
	Status      string     `json:"status"`
	Notes       *string    `json:"notes,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateRoomInput is the body for POST /owner/rooms.
// Status defaults to "available" when omitted.
type CreateRoomInput struct {
	RoomNumber  string `json:"room_number"  binding:"required"`
	RoomName    string `json:"room_name"`
	MonthlyRent int    `json:"monthly_rent"  binding:"required,min=1"`
	Status      string `json:"status"        binding:"omitempty,oneof=available maintenance inactive"`
	Notes       string `json:"notes"`
}

// UpdateRoomInput is the body for PATCH /owner/rooms/{id}. All fields are optional.
type UpdateRoomInput struct {
	RoomNumber  *string `json:"room_number"  binding:"omitempty,min=1"`
	RoomName    *string `json:"room_name"`
	MonthlyRent *int    `json:"monthly_rent"  binding:"omitempty,min=1"`
	Status      *string `json:"status"        binding:"omitempty,oneof=available reserved occupied maintenance inactive"`
	Notes       *string `json:"notes"`
}

// ListRoomsFilter carries query params for GET /owner/rooms.
type ListRoomsFilter struct {
	Status string
	Search string
	Page   int
	Limit  int
}

// ListRoomsResult is the paginated list response.
type ListRoomsResult struct {
	Rooms []*Room `json:"rooms"`
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Limit int     `json:"limit"`
}

// RoomDetailView is the response for GET /owner/rooms/{room_id}/detail.
type RoomDetailView struct {
	Room          *Room              `json:"room"`
	CurrentTenant *RoomCurrentTenant `json:"current_tenant"`
	BillHistory   *ListBillsResult   `json:"bill_history"`
}

// RoomCurrentTenant is the tenant on the room's active/pending assignment,
// flattened with assignment terms. Nil when the room is vacant (BR-010/BR-011:
// at most one active/pending assignment per room).
type RoomCurrentTenant struct {
	TenantID         string     `json:"tenant_id"`
	FullName         string     `json:"full_name"`
	PhoneNumber      *string    `json:"phone_number,omitempty"`
	Email            *string    `json:"email,omitempty"`
	RoomAssignmentID string     `json:"room_assignment_id"`
	AssignmentStatus string     `json:"assignment_status"`
	StartDate        time.Time  `json:"start_date"`
	EndDate          *time.Time `json:"end_date,omitempty"`
	MonthlyRent      int        `json:"monthly_rent"`
	PaymentDueDay    int        `json:"payment_due_day"`
}

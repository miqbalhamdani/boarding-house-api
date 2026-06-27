package model

import "time"

// RoomAssignment connects a tenant to a room for a billing arrangement.
type RoomAssignment struct {
	ID            string     `json:"id"`
	OwnerID       string     `json:"owner_id"`
	TenantID      string     `json:"tenant_id"`
	RoomID        string     `json:"room_id"`
	StartDate     time.Time  `json:"start_date"`
	EndDate       *time.Time `json:"end_date,omitempty"`
	MonthlyRent   int        `json:"monthly_rent"`
	PaymentDueDay int        `json:"payment_due_day"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Bill is a monthly rent bill. Onboarding creates the first one.
type Bill struct {
	ID                 string     `json:"id"`
	OwnerID            string     `json:"owner_id"`
	TenantID           string     `json:"tenant_id"`
	RoomID             string     `json:"room_id"`
	RoomAssignmentID   string     `json:"room_assignment_id"`
	BillNumber         string     `json:"bill_number"`
	BillType           string     `json:"bill_type"`
	BillingMonth       string     `json:"billing_month"`
	BillingPeriodStart time.Time  `json:"billing_period_start"`
	BillingPeriodEnd   time.Time  `json:"billing_period_end"`
	Amount             int        `json:"amount"`
	DueDate            time.Time  `json:"due_date"`
	Status             string     `json:"status"`
	PaidAt             *time.Time `json:"paid_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AssignRoomInput is the body for POST /owner/onboarding/assign-room.
// owner_id is never accepted from the body; it is derived from the owner token.
type AssignRoomInput struct {
	TenantID      string `json:"tenant_id"       binding:"required,uuid"`
	RoomID        string `json:"room_id"         binding:"required,uuid"`
	StartDate     string `json:"start_date"      binding:"required"`
	MonthlyRent   int    `json:"monthly_rent"    binding:"required,min=1"`
	PaymentDueDay int    `json:"payment_due_day" binding:"required,min=1,max=31"`
}

// AssignRoomResult is the response for a successful onboarding.
type AssignRoomResult struct {
	RoomAssignmentID string `json:"room_assignment_id"`
	FirstBillID      string `json:"first_bill_id"`
	TenantStatus     string `json:"tenant_status"`
	RoomStatus       string `json:"room_status"`
}

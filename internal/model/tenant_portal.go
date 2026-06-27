package model

import "time"

// TenantRoomView is the GET /tenant/my-room response: the tenant's current room
// assignment flattened with the room it points to. Only the active or pending
// assignment is returned; ownership is enforced by the query, not the payload.
type TenantRoomView struct {
	RoomAssignmentID string     `json:"room_assignment_id"`
	AssignmentStatus string     `json:"assignment_status"`
	StartDate        time.Time  `json:"start_date"`
	EndDate          *time.Time `json:"end_date,omitempty"`
	MonthlyRent      int        `json:"monthly_rent"`
	PaymentDueDay    int        `json:"payment_due_day"`
	RoomID           string     `json:"room_id"`
	RoomNumber       string     `json:"room_number"`
	RoomName         *string    `json:"room_name,omitempty"`
	RoomStatus       string     `json:"room_status"`
	Notes            *string    `json:"notes,omitempty"`
}

// TenantListBillsFilter carries query params for GET /tenant/bills. The tenant
// scope (tenant_id, owner_id) is always derived from the token, never the query.
type TenantListBillsFilter struct {
	Status string
	Page   int
	Limit  int
}

// TenantListPaymentsFilter carries query params for GET /tenant/payments.
type TenantListPaymentsFilter struct {
	Page  int
	Limit int
}

// PayBillInput is the body for POST /tenant/bills/{bill_id}/pay. provider is
// optional; when omitted the configured default gateway provider is used. The
// amount is never accepted here — it is always the bill amount (BR-019, BR-022).
type PayBillInput struct {
	Provider string `json:"provider" binding:"omitempty"`
}

// PayBillResult is returned when a tenant opens a checkout link for a bill.
type PayBillResult struct {
	GatewayTransactionID string     `json:"gateway_transaction_id"`
	Provider             string     `json:"provider"`
	CheckoutURL          string     `json:"checkout_url"`
	Status               string     `json:"status"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
}

// GatewayTransaction is a single checkout/payment attempt created through a
// payment gateway provider. Module 08 only creates pending transactions; the
// webhook-driven status transitions belong to the payment-gateway module.
type GatewayTransaction struct {
	ID                    string     `json:"id"`
	OwnerID               string     `json:"owner_id"`
	BillID                string     `json:"bill_id"`
	TenantID              string     `json:"tenant_id"`
	Provider              string     `json:"provider"`
	ExternalTransactionID *string    `json:"external_transaction_id,omitempty"`
	ExternalOrderID       string     `json:"external_order_id"`
	CheckoutURL           *string    `json:"checkout_url,omitempty"`
	Amount                int        `json:"amount"`
	Currency              string     `json:"currency"`
	Status                string     `json:"status"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	PaidAt                *time.Time `json:"paid_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

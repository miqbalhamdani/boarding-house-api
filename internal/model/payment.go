package model

import "time"

// Payment is a successful full payment for a bill (Module 06). The MVP only
// records full payments; partial payments and refunds are out of scope.
type Payment struct {
	ID                   string    `json:"id"`
	OwnerID              string    `json:"owner_id"`
	BillID               string    `json:"bill_id"`
	TenantID             string    `json:"tenant_id"`
	RoomID               string    `json:"room_id"`
	Amount               int       `json:"amount"`
	PaymentDate          time.Time `json:"payment_date"`
	PaymentMethod        string    `json:"payment_method"`
	PaymentSource        string    `json:"payment_source"`
	GatewayTransactionID *string   `json:"gateway_transaction_id,omitempty"`
	ReferenceNumber      *string   `json:"reference_number,omitempty"`
	Notes                *string   `json:"notes,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// RecordManualPaymentInput is the body for POST /owner/payments/manual.
// owner_id is never accepted from the body; it is derived from the owner token.
// payment_date is optional; when omitted the server's current time is used.
type RecordManualPaymentInput struct {
	BillID          string `json:"bill_id"          binding:"required,uuid"`
	Amount          int    `json:"amount"           binding:"required,min=1"`
	PaymentDate     string `json:"payment_date"     binding:"omitempty"`
	PaymentMethod   string `json:"payment_method"   binding:"required"`
	ReferenceNumber string `json:"reference_number" binding:"omitempty,max=150"`
	Notes           string `json:"notes"            binding:"omitempty"`
}

// ListPaymentsFilter carries query params for GET /owner/payments.
type ListPaymentsFilter struct {
	TenantID string
	Month    string // YYYY-MM, filters by payment_date month
	Page     int
	Limit    int
}

// ListPaymentsResult is the paginated payment list response.
type ListPaymentsResult struct {
	Payments []*Payment `json:"payments"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	Limit    int        `json:"limit"`
}

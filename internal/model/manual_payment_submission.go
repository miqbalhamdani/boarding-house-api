package model

import "time"

// ManualPaymentSubmission is a tenant's claim that they paid a bill outside the
// application (Module 10). It is a payment *claim*, not a payment: it never
// marks the bill paid on its own. Only owner approval creates a payment.
type ManualPaymentSubmission struct {
	ID                string    `json:"id"`
	OwnerID           string    `json:"owner_id"`
	BillID            string    `json:"bill_id"`
	TenantID          string    `json:"tenant_id"`
	SubmittedAmount   int       `json:"submitted_amount"`
	PaymentMethod     string    `json:"payment_method"`
	TransferDate      time.Time `json:"transfer_date"`
	SenderAccountName *string   `json:"sender_account_name,omitempty"`
	ReferenceNumber   *string   `json:"reference_number,omitempty"`
	// ProofURL holds the private object-store key. Never serialized: viewers get
	// a short-lived presigned URL (ProofViewURL) instead.
	ProofURL     *string    `json:"-"`
	Status       string     `json:"status"`
	TenantNotes  *string    `json:"tenant_notes,omitempty"`
	ReviewReason *string    `json:"review_reason,omitempty"`
	ReviewNotes  *string    `json:"review_notes,omitempty"`
	ReviewedBy   *string    `json:"reviewed_by,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// Denormalized fields joined on read for owner list/detail (zero on insert),
	// mirroring the Bill.TenantName/RoomNumber pattern.
	TenantName   string `json:"tenant_name,omitempty"`
	RoomNumber   string `json:"room_number,omitempty"`
	BillAmount   int    `json:"bill_amount,omitempty"`
	BillingMonth string `json:"billing_month,omitempty"`
	// ProofViewURL is a short-lived presigned URL, populated only on owner detail.
	ProofViewURL string `json:"proof_view_url,omitempty"`
}

// SubmitManualPaymentInput is the parsed multipart body for
// POST /tenant/bills/{bill_id}/manual-payment-submissions. It is NOT a JSON
// binding DTO; the handler parses multipart form fields and the proof file.
// owner_id/tenant_id/bill_id are never taken from the body.
type SubmitManualPaymentInput struct {
	SubmittedAmount   int
	PaymentMethod     string
	TransferDate      string // RFC3339, parsed in the service
	SenderAccountName string
	ReferenceNumber   string
	Notes             string
	ProofFileName     string // original name, informational only — never trusted for path/ext
	ProofContent      []byte
}

// ReviewSubmissionInput is the body for POST .../approve.
type ReviewSubmissionInput struct {
	ReviewNotes string `json:"review_notes" binding:"omitempty"`
}

// RejectSubmissionInput is the body for POST .../reject. A reason is required.
type RejectSubmissionInput struct {
	Reason      string `json:"reason"       binding:"required,max=80"`
	ReviewNotes string `json:"review_notes" binding:"omitempty"`
}

// ListManualPaymentSubmissionsFilter carries query params for the owner list.
type ListManualPaymentSubmissionsFilter struct {
	Status       string
	TenantID     string
	BillingMonth string // YYYY-MM, matched against bills.billing_month
	Page         int
	Limit        int
}

// ListManualPaymentSubmissionsResult is the paginated owner list response.
type ListManualPaymentSubmissionsResult struct {
	Submissions []*ManualPaymentSubmission `json:"submissions"`
	Total       int                        `json:"total"`
	Page        int                        `json:"page"`
	Limit       int                        `json:"limit"`
}

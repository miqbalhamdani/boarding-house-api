package model

// ListBillsFilter carries query params for GET /owner/bills.
type ListBillsFilter struct {
	Status       string
	BillingMonth string
	TenantID     string
	RoomID       string
	Page         int
	Limit        int
	// SortByDueDate orders results by due_date DESC (list endpoint only).
	SortByDueDate bool
}

// ListBillsResult is the paginated bill list response.
type ListBillsResult struct {
	Bills []*Bill `json:"bills"`
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Limit int     `json:"limit"`
}

// GenerateMonthlyInput is the body for POST /owner/bills/generate-monthly.
// billing_month is optional; when omitted the current calendar month is used.
// owner_id is never accepted from the body; it is derived from the owner token.
type GenerateMonthlyInput struct {
	BillingMonth string `json:"billing_month" binding:"omitempty,len=7"`
}

// GenerateMonthlyResult summarises an idempotent monthly generation run.
type GenerateMonthlyResult struct {
	BillingMonth     string `json:"billing_month"`
	ActiveAssignment int    `json:"active_assignments"`
	Created          int    `json:"created"`
	Skipped          int    `json:"skipped"`
}

// MarkOverdueResult reports how many bills transitioned to overdue.
type MarkOverdueResult struct {
	Updated int `json:"updated"`
}

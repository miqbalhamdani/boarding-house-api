package model

// DashboardSummary is the owner overview returned by
// GET /owner/dashboard/summary (Module 07). All metrics are scoped to a single
// owner (BR-031).
//
// The room/tenant/bill counts reflect current state; paid_bills_this_month and
// collected_amount_this_month are scoped to the requested calendar month and
// count only successful payments (BR-031, dashboard rule: gateway pending does
// not count as collected).
type DashboardSummary struct {
	TotalRooms               int `json:"total_rooms"`
	AvailableRooms           int `json:"available_rooms"`
	OccupiedRooms            int `json:"occupied_rooms"`
	ActiveTenants            int `json:"active_tenants"`
	UnpaidBills              int `json:"unpaid_bills"`
	OverdueBills             int `json:"overdue_bills"`
	GatewayPendingBills      int `json:"gateway_pending_bills"`
	PaidBillsThisMonth       int `json:"paid_bills_this_month"`
	CollectedAmountThisMonth int `json:"collected_amount_this_month"`
}

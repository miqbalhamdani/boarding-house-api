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

// DashboardView is the enriched owner overview returned by
// GET /owner/dashboard/summary. It embeds the flat DashboardSummary counts (so
// those keys stay at the top level of the response) and adds short, owner-scoped
// preview lists that mirror the "embedded list" enrichment used by the room and
// tenant detail endpoints.
//
// The bill lists reflect current outstanding state (status filter only, not
// month-bound), matching the outstanding-bill counts. RecentPayments is scoped
// to the requested calendar month, matching PaidBillsThisMonth and
// CollectedAmountThisMonth. Each list is capped to a small preview size; its
// Total field still reports the full owner-scoped count.
type DashboardView struct {
	*DashboardSummary
	UnpaidBills         *ListBillsResult    `json:"unpaid_bills_list"`
	OverdueBills        *ListBillsResult    `json:"overdue_bills_list"`
	GatewayPendingBills *ListBillsResult    `json:"gateway_pending_bills_list"`
	RecentPayments      *ListPaymentsResult `json:"recent_payments"`
}

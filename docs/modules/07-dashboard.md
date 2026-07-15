# Module 07: Dashboard

## Goal
Give owner a clear overview of rooms, tenants, bills, payments, and gateway status.

## Scope
Included:
- dashboard summary cards
- unpaid bills list
- overdue bills list
- gateway pending list
- recent payments list

Excluded:
- advanced analytics
- accounting export
- charts beyond MVP

## API Endpoints
- `GET /owner/dashboard/summary?month=YYYY-MM`

## Metrics
- total rooms
- available rooms
- occupied rooms
- active tenants
- unpaid bills
- overdue bills
- gateway pending bills
- paid bills this month
- collected amount this month

## Embedded Lists
The summary endpoint also returns short preview lists (capped at 5 rows each;
each list's `total` reports the full owner-scoped count):
- `unpaid_bills_list`, `overdue_bills_list`, `gateway_pending_bills_list`:
  current outstanding bills by status (not month-bound).
- `recent_payments`: successful payments within the requested month.

## Business Rules
- Dashboard data must be scoped by owner ID.
- Collected amount only counts successful payments.
- Gateway pending does not count as collected.

## Acceptance Criteria
- Owner sees accurate dashboard data.
- Dashboard excludes other owners' data.
- Dashboard highlights overdue and gateway pending bills.

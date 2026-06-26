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

## Business Rules
- Dashboard data must be scoped by owner ID.
- Collected amount only counts successful payments.
- Gateway pending does not count as collected.

## Acceptance Criteria
- Owner sees accurate dashboard data.
- Dashboard excludes other owners' data.
- Dashboard highlights overdue and gateway pending bills.

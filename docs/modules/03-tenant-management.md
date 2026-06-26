# Module 03: Tenant Management

## Goal
Allow owner to manage tenant profiles.

## Scope
Included:
- tenant list
- tenant create
- tenant detail
- tenant update
- soft delete tenant
- status filter

Excluded:
- document upload
- tenant contract signing
- tenant KYC verification

## Main Table
- tenants

## API Endpoints
- `GET /owner/tenants`
- `POST /owner/tenants`
- `GET /owner/tenants/{tenant_id}`
- `PATCH /owner/tenants/{tenant_id}`

## Business Rules
- Tenant belongs to one owner.
- Tenant status options: pending_payment, active, moved_out, cancelled.
- Tenant becomes active only after first rent bill is paid.
- Tenant portal credentials may be generated during tenant creation or later.

## Acceptance Criteria
- Owner can create and update tenants.
- Owner cannot access another owner's tenants.
- Tenant detail shows current assignment and billing summary.

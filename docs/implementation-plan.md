# Implementation Plan

## Recommended Build Order

Build one module at a time. Do not build the entire app at once.

## Phase 1: Foundation

### Step 1: Project Setup
- Create frontend app.
- Create backend app.
- Configure database.
- Configure environment variables.
- Add request validation.
- Add error response format.
- Add authentication foundation.

### Step 2: Auth Module
Read:
- `/docs/product-requirements.md`
- `/docs/database-schema.md`
- `/docs/business-rules.md`
- `/docs/modules/01-auth.md`

Build:
- owner register
- owner login
- tenant login
- auth guards
- owner isolation helper

## Phase 2: Core Rental Management

### Step 3: Room Management
Read:
- `/docs/modules/02-room-management.md`

Build:
- room CRUD
- room list filters
- room status validation

### Step 4: Tenant Management
Read:
- `/docs/modules/03-tenant-management.md`

Build:
- tenant CRUD
- tenant list filters
- tenant detail

### Step 5: Tenant Onboarding
Read:
- `/docs/modules/04-tenant-onboarding.md`

Build:
- assign tenant to room
- create room assignment
- create first rent bill
- update room status to reserved

## Phase 3: Billing and Payments

### Step 6: Monthly Billing
Read:
- `/docs/modules/05-monthly-billing.md`

Build:
- bill list
- bill detail
- automatic bill generation job
- manual generate bills backup action
- overdue marking job

### Step 7: Payment Gateway
Read:
- `/docs/modules/09-payment-gateway.md`

Build:
- provider abstraction
- create checkout link
- store gateway transaction
- webhook endpoint
- signature verification
- idempotent webhook processing
- gateway transaction status updates

### Step 8: Payment Tracking
Read:
- `/docs/modules/06-payment-tracking.md`

Build:
- manual full payment record
- successful gateway payment record creation
- bill paid transition
- first payment activation logic

## Phase 4: Dashboard and Tenant Portal

### Step 9: Dashboard
Read:
- `/docs/modules/07-dashboard.md`

Build:
- dashboard summary API
- dashboard UI cards
- unpaid/overdue/gateway pending lists

### Step 10: Tenant Portal
Read:
- `/docs/modules/08-tenant-portal.md`

Build:
- tenant dashboard
- tenant bills
- Pay Now button
- payment result page
- tenant payment history

## Phase 5: Hardening

### Step 11: Security and Reliability
- Add tests for owner isolation.
- Add tests for duplicate bill prevention.
- Add tests for full-payment-only validation.
- Add tests for idempotent gateway webhooks.
- Add logs for gateway webhook failures.
- Add transaction locks around payment creation.

## Copilot Build Instruction Template

Use this when asking Copilot/AI to build one module:

```text
Read these files first:
- /docs/product-requirements.md
- /docs/business-rules.md
- /docs/database-schema.md
- /docs/api-spec.md
- /docs/ui-pages.md
- /docs/user-flows.md
- /docs/coding-rules.md
- /docs/modules/<module-file>.md

Task:
Build only <module name>.

Rules:
- Do not build other modules.
- Do not change unrelated files.
- Follow the API spec.
- Follow the database schema.
- Enforce owner_id isolation.
- After finishing, list changed files and explain what was implemented.
```

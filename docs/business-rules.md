# Business Rules

## 1. Multi-Tenant Rules

### BR-001 Owner Isolation
Every owner can only access data where `owner_id` equals their authenticated owner ID.

Applies to:
- rooms
- tenants
- room_assignments
- bills
- payments
- payment_gateway_transactions
- payment_gateway_webhook_events

### BR-002 Tenant Access
A tenant can only access their own tenant portal data.

A tenant cannot access:
- other tenants
- owner dashboard
- owner room management
- owner payment reports

## 2. Room Rules

### BR-003 Room Status
Allowed room statuses:
- `available`
- `reserved`
- `occupied`
- `maintenance`
- `inactive`

### BR-004 Available Room Assignment
Only rooms with status `available` can be assigned to a new tenant.

### BR-005 Reserved During Onboarding
After tenant onboarding is created but before first rent payment is completed, room status becomes `reserved`.

### BR-006 Occupied After First Payment
After first rent bill is paid, room status becomes `occupied`.

## 3. Tenant Rules

### BR-007 Tenant Status
Allowed tenant statuses:
- `pending_payment`
- `active`
- `moved_out`
- `cancelled`

### BR-008 Tenant Activation
A tenant becomes `active` only after the first rent bill is paid in full.

### BR-009 Tenant Move Out
When a tenant moves out:
- active room assignment becomes `ended`
- tenant status becomes `moved_out`
- room status becomes `available` or `maintenance` based on owner choice
- future unpaid bills may be cancelled manually if they should no longer be collected

## 4. Room Assignment Rules

### BR-010 One Active Assignment Per Room
A room can only have one active or pending assignment at a time.

### BR-011 One Active Assignment Per Tenant
A tenant can only have one active or pending room assignment at a time.

### BR-012 Rent Snapshot
`room_assignments.monthly_rent` stores the rent amount used for billing that tenant. It must not automatically change when `rooms.monthly_rent` changes later.

## 5. Billing Rules

### BR-013 Monthly Rent Only
The MVP only supports monthly rent bills.

Excluded:
- deposit bill
- utility bill
- maintenance bill
- late fee bill

### BR-014 First Bill During Onboarding
When a tenant is assigned to a room, the system creates the first monthly rent bill immediately.

### BR-015 Automatic Monthly Bill Generation
The system automatically generates monthly rent bills for all active room assignments.

Recommended MVP schedule:
- run once per day
- on the first day of the month, create bills for the current billing month

### BR-016 No Duplicate Monthly Bills
The system must not create duplicate bills for the same `room_assignment_id` and `billing_month`.

Database should enforce:

```text
unique(room_assignment_id, billing_month)
```

### BR-017 Bill Status
Allowed bill statuses:
- `unpaid`
- `gateway_pending`
- `paid`
- `overdue`
- `cancelled`

### BR-018 Overdue Bill
If current date is after `due_date` and bill status is `unpaid`, bill becomes `overdue`.

A bill with status `gateway_pending` may also become `overdue` if the checkout expires and due date has passed.

### BR-019 Full Amount Only
A bill must be paid in full. Partial payments are not allowed.

Payment amount must equal bill amount.

## 6. Payment Gateway Rules

### BR-020 Payment Link Creation
A payment gateway checkout link can only be created for bills with status:
- `unpaid`
- `overdue`
- `gateway_pending` with expired or failed transaction

### BR-021 One Active Gateway Transaction Per Bill
A bill can have multiple gateway transaction attempts over time, but only one active pending transaction at a time.

### BR-022 Gateway Amount Validation
Gateway transaction amount must equal bill amount.

### BR-023 Bill Gateway Pending
When a checkout link is created, the bill status becomes `gateway_pending`.

### BR-024 Webhook Verification
All gateway webhooks must be verified using the provider signature or security mechanism before processing.

### BR-025 Idempotent Webhooks
Webhook processing must be idempotent.

If the same webhook event is received multiple times, the system must not create duplicate payment records.

### BR-026 Paid Status Source of Truth
For gateway payments, the system marks a bill as paid only after a verified webhook confirms successful payment.

### BR-027 Gateway Failure
If gateway payment fails, expires, or is cancelled:
- gateway transaction status is updated
- bill returns to `unpaid` or `overdue` depending on due date
- tenant may generate a new payment link

### BR-028 Manual Payment Backup
Owner can manually record a full payment if the tenant pays outside the gateway.

Manual payment is only allowed when bill is not already paid.

### BR-029 Payment Record Creation
A payment record is created when:
- owner records manual full payment, or
- verified gateway webhook confirms successful full payment

### BR-030 One Successful Payment Per Bill
Each bill can only have one successful payment.

Database should enforce one successful payment per bill with a unique constraint or application-level transaction lock.

## 7. Dashboard Rules

### BR-031 Dashboard Metrics
Dashboard numbers must be calculated per owner only.

Metrics:
- total rooms
- available rooms
- occupied rooms
- active tenants
- unpaid bills
- overdue bills
- gateway pending bills
- paid bills this month
- collected amount this month

## 8. Audit and Security Rules

### BR-032 Timestamps
All main records must include:
- `created_at`
- `updated_at`

### BR-033 Soft Delete Recommended
Use soft delete for business records where historical data matters.

Recommended soft delete fields:
- `deleted_at`

### BR-034 Gateway Event Storage
Store raw gateway webhook events for audit, debugging, and reconciliation.

Sensitive values should not be logged in plain text if prohibited by the gateway provider.

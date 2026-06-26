# Module 06: Payment Tracking

## Goal
Record successful full payments and update bills.

## Scope
Included:
- manual full payment recording
- gateway successful payment recording
- payment list
- payment detail
- bill paid status transition
- first payment activation logic

Excluded:
- partial payments
- refunds
- payment allocation across multiple bills

## Main Tables
- bills
- payments
- payment_gateway_transactions

## API Endpoints
- `POST /owner/payments/manual`
- `GET /owner/payments`
- `GET /owner/payments/{payment_id}`

## Business Rules
- Payment amount must equal bill amount.
- Bill must not already be paid.
- One bill can only have one successful payment.
- Manual payment source is `manual`.
- Gateway payment source is `gateway`.
- Payment creation and bill update must be in one database transaction.
- If paid bill is the first bill for pending assignment, activate tenant, assignment, and room.

## Acceptance Criteria
- Owner can record manual full payment.
- Partial amount is rejected.
- Duplicate payment is rejected.
- Gateway success creates payment once.
- Bill becomes paid after payment.

# Module 08: Tenant Portal

## Goal
Allow tenant to view assigned room, bills, payment links, and payment history.

## Scope
Included:
- tenant login
- tenant dashboard
- my room
- my bills
- bill detail
- Pay Now button
- payment result page
- payment history

Excluded:
- tenant profile editing
- maintenance requests
- chat/support

## API Endpoints
- `GET /tenant/me`
- `GET /tenant/my-room`
- `GET /tenant/bills`
- `GET /tenant/bills/{bill_id}`
- `POST /tenant/bills/{bill_id}/pay`
- `GET /tenant/payments`

## Business Rules
- Tenant can only see their own data.
- Tenant can only pay unpaid, overdue, or eligible gateway pending bills.
- Tenant cannot manually mark payment as paid.
- Frontend redirect from gateway is not proof of payment.
- Tenant bill status updates after verified webhook processing.

## Acceptance Criteria
- Tenant can view assigned room.
- Tenant can view bills.
- Tenant can click Pay Now and receive checkout URL.
- Tenant can view payment history after successful payment.

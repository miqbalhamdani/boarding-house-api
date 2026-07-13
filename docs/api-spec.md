# API Specification

## API Style
REST JSON API.

Base path:

```text
/api/v1
```

## Authentication
Owner endpoints require owner access token.
Tenant portal endpoints require tenant access token.
Payment webhook endpoint uses gateway signature verification, not user JWT.

Example header:

```http
Authorization: Bearer <token>
```

## Common Response Format

### Success
```json
{
  "data": {},
  "message": "Success"
}
```

### Error
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Payment amount must equal bill amount",
    "fields": {}
  }
}
```

## Auth API

### Register Owner
`POST /auth/owner/register`

Request:
```json
{
  "business_name": "Kos Budi",
  "full_name": "Owner Name",
  "email": "owner@example.com",
  "password": "password123",
  "phone_number": "08123456789"
}
```

### Login Owner
`POST /auth/owner/login`

### Login Tenant
`POST /auth/tenant/login`

### Owner Profile (Me)
`GET /owner/me`

Requires an owner access token. `owner_id` and `owner_user_id` are derived from
the token — never from query or body. Returns the profile only; no auth tokens.

Response:
```json
{
  "data": {
    "owner_id": "uuid",
    "owner_user_id": "uuid",
    "business_name": "Kos Budi",
    "full_name": "Owner Name",
    "email": "owner@example.com",
    "phone_number": "08123456789",
    "status": "active"
  },
  "message": "Success"
}
```

## Room API

### List Rooms
`GET /owner/rooms?status=available&search=101&page=1&limit=20`

### Create Room
`POST /owner/rooms`

Request:
```json
{
  "room_number": "101",
  "room_name": "Room 101",
  "monthly_rent": 2000000,
  "status": "available",
  "notes": "Near front door"
}
```

### Get Room Detail
`GET /owner/rooms/{room_id}?status=unpaid&page=1&limit=20`

Returns the room plus its current tenant (null when vacant) and paginated bill history for the room. Query params (`status`, `page`, `limit`) filter/paginate `bill_history` only.

Response:
```json
{
  "data": {
    "room": {
      "id": "...",
      "room_number": "101",
      "room_name": "Room 101",
      "monthly_rent": 2000000,
      "status": "occupied",
      "notes": null,
      "created_at": "...",
      "updated_at": "..."
    },
    "current_tenant": {
      "tenant_id": "...",
      "full_name": "Budi Santoso",
      "phone_number": "0812...",
      "email": "budi@example.com",
      "room_assignment_id": "...",
      "assignment_status": "active",
      "start_date": "...",
      "end_date": null,
      "monthly_rent": 2000000,
      "payment_due_day": 5
    },
    "bill_history": {
      "bills": [ { "id": "...", "billing_month": "2026-07", "amount": 2000000, "status": "unpaid", "...": "..." } ],
      "total": 12,
      "page": 1,
      "limit": 20
    }
  },
  "message": "Success"
}
```

### Update Room
`PATCH /owner/rooms/{room_id}`

### Delete Room
`DELETE /owner/rooms/{room_id}`

## Tenant API

### List Tenants
`GET /owner/tenants?status=active&search=budi&page=1&limit=20`

### Create Tenant
`POST /owner/tenants`

Request:
```json
{
  "full_name": "Budi Santoso",
  "phone_number": "081234567890",
  "email": "budi@example.com",
  "identity_number": "317300001",
  "emergency_contact_name": "Siti",
  "emergency_contact_phone": "081299988877"
}
```

### Get Tenant Detail
`GET /owner/tenants/{tenant_id}`

### Update Tenant
`PATCH /owner/tenants/{tenant_id}`

## Tenant Onboarding API

### Assign Tenant to Room
`POST /owner/onboarding/assign-room`

Request:
```json
{
  "tenant_id": "uuid",
  "room_id": "uuid",
  "start_date": "2026-07-10",
  "monthly_rent": 2000000,
  "payment_due_day": 10
}
```

Response:
```json
{
  "data": {
    "room_assignment_id": "uuid",
    "first_bill_id": "uuid",
    "tenant_status": "pending_payment",
    "room_status": "reserved"
  }
}
```

### Cancel Onboarding
`POST /owner/onboarding/{room_assignment_id}/cancel`

## Billing API

### List Bills
`GET /owner/bills?status=unpaid&billing_month=2026-07&page=1&limit=20`

### Get Bill Detail
`GET /owner/bills/{bill_id}`

### Generate Monthly Bills Manually
`POST /owner/bills/generate-monthly`

Request:
```json
{
  "billing_month": "2026-07"
}
```

This is a backup endpoint. A scheduled job should generate bills automatically.

### Mark Overdue Bills
`POST /owner/bills/mark-overdue`

Optional admin/system endpoint to update overdue bill statuses.

## Payment Tracking API

### Record Manual Full Payment
`POST /owner/payments/manual`

Request:
```json
{
  "bill_id": "uuid",
  "amount": 2000000,
  "payment_date": "2026-07-10T10:00:00Z",
  "payment_method": "bank_transfer",
  "reference_number": "TRX-001",
  "notes": "Paid by bank transfer"
}
```

Rules:
- amount must equal bill amount
- bill must not already be paid
- bill must not have a successful payment

### List Payments
`GET /owner/payments?tenant_id=uuid&month=2026-07&page=1&limit=20`

### Get Payment Detail
`GET /owner/payments/{payment_id}`

## Payment Gateway API

### Create Checkout Link for Bill
`POST /owner/bills/{bill_id}/gateway-checkout`

Owner can create or refresh a payment link for a tenant.

Request:
```json
{
  "provider": "midtrans"
}
```

Response:
```json
{
  "data": {
    "gateway_transaction_id": "uuid",
    "provider": "midtrans",
    "checkout_url": "https://gateway.example/checkout/abc",
    "status": "pending",
    "expires_at": "2026-07-10T23:59:59Z"
  }
}
```

### Tenant Create Checkout Link
`POST /tenant/bills/{bill_id}/pay`

Tenant creates checkout for their own unpaid bill.

Request:
```json
{
  "provider": "midtrans"
}
```

Response:
```json
{
  "data": {
    "checkout_url": "https://gateway.example/checkout/abc",
    "status": "pending"
  }
}
```

### Get Gateway Transaction Status
`GET /owner/gateway-transactions/{gateway_transaction_id}`

### Payment Gateway Webhook
`POST /webhooks/payment-gateway/{provider}`

This endpoint receives payment notifications from gateway providers.

Important implementation requirements:
- verify provider signature
- store raw payload
- process idempotently
- update gateway transaction status
- create payment record only once
- mark bill as paid only for successful payment status

Generic response:
```json
{
  "message": "Webhook received"
}
```

## Dashboard API

### Owner Dashboard Summary
`GET /owner/dashboard/summary?month=2026-07`

Response:
```json
{
  "data": {
    "total_rooms": 10,
    "available_rooms": 3,
    "occupied_rooms": 7,
    "active_tenants": 7,
    "unpaid_bills": 2,
    "overdue_bills": 1,
    "gateway_pending_bills": 1,
    "paid_bills_this_month": 5,
    "collected_amount_this_month": 10000000
  }
}
```

## Tenant Portal API

### My Profile
`GET /tenant/me`

### My Room Assignment
`GET /tenant/my-room`

### My Bills
`GET /tenant/bills?status=unpaid&page=1&limit=20`

### My Bill Detail
`GET /tenant/bills/{bill_id}`

### My Payments
`GET /tenant/payments?page=1&limit=20`

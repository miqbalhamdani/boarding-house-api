# Module 09: Payment Gateway

## Goal
Integrate payment gateway checkout so tenants can pay full monthly rent bills online.

## Scope
Included:
- provider-neutral payment gateway abstraction
- create checkout/payment link for bill
- store gateway transaction attempts
- tenant Pay Now flow
- owner create/copy payment link
- webhook endpoint
- signature verification
- idempotent webhook processing
- update bill and payment after successful webhook

Excluded:
- partial payments
- refunds
- recurring auto-charge
- payout settlement reconciliation
- multiple split recipients

## Main Tables
- bills
- payments
- payment_gateway_transactions
- payment_gateway_webhook_events

## API Endpoints
- `POST /owner/bills/{bill_id}/gateway-checkout`
- `POST /tenant/bills/{bill_id}/pay`
- `GET /owner/gateway-transactions/{gateway_transaction_id}`
- `POST /webhooks/payment-gateway/{provider}`

## Provider Abstraction
Create an internal payment provider interface:

```text
createCheckout(input) -> checkout_url, external_order_id, external_transaction_id, expires_at, raw_response
verifyWebhook(headers, rawBody) -> valid/invalid
parseWebhook(rawBody) -> normalized event
```

Normalized webhook event:

```json
{
  "provider": "midtrans",
  "external_order_id": "BILL-2026-07-001",
  "external_transaction_id": "trx-123",
  "status": "paid",
  "amount": 2000000,
  "paid_at": "2026-07-10T10:00:00Z",
  "event_id": "evt-123"
}
```

## Checkout Creation Flow

```text
User clicks Pay Now
→ system validates bill belongs to user/owner
→ system validates bill is not paid/cancelled
→ system validates amount equals bill amount
→ system creates external order ID
→ system calls gateway create checkout API
→ system stores payment_gateway_transactions record
→ system changes bill status to gateway_pending
→ system returns checkout URL
```

## Webhook Success Flow

```text
Gateway sends webhook
→ system stores webhook event
→ system verifies signature
→ system checks event has not been processed
→ system finds gateway transaction by external_order_id or external_transaction_id
→ system validates amount equals bill amount
→ system updates gateway transaction to paid
→ system creates payment record with source gateway
→ system marks bill as paid
→ if first bill, activate tenant, room assignment, and room
→ system marks webhook event processed
```

## Failure/Expired Flow

```text
Gateway sends failed/expired/cancelled webhook
→ system verifies signature
→ system updates gateway transaction status
→ if bill is not paid, bill returns to unpaid or overdue
→ tenant can create a new checkout link
```

## Business Rules
- Only unpaid, overdue, or eligible expired gateway pending bills can create checkout.
- One active pending gateway transaction per bill.
- Gateway amount must equal bill amount.
- Webhook must be verified before processing.
- Webhook processing must be idempotent.
- Do not trust frontend redirect as payment success.
- Only verified gateway webhook can create gateway payment record.

## Environment Variables
Example names:

```text
PAYMENT_GATEWAY_PROVIDER=midtrans
PAYMENT_GATEWAY_SERVER_KEY=...
PAYMENT_GATEWAY_CLIENT_KEY=...
PAYMENT_GATEWAY_WEBHOOK_SECRET=...
PAYMENT_GATEWAY_RETURN_URL=https://app.example.com/tenant/payment-result
```

## Acceptance Criteria
- Tenant can create checkout link for unpaid bill.
- Owner can create/copy checkout link for unpaid bill.
- Gateway transaction is stored.
- Webhook signature is verified.
- Successful webhook marks bill paid.
- Duplicate webhook does not duplicate payment.
- Failed/expired webhook does not mark bill paid.

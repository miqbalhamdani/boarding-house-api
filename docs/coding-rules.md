# Coding Rules

## General Rules
- Build module by module.
- Do not implement features outside the selected module.
- Keep business logic in service/use-case layer.
- Keep controllers thin.
- Validate all request input.
- Return consistent API response format.
- Use clear error codes.

## Multi-Tenant Rules
- Every owner-owned query must filter by `owner_id`.
- Never accept `owner_id` from request body.
- Always derive `owner_id` from authenticated owner token.
- Tenant portal queries must derive `tenant_id` from tenant token.
- Add tests to prevent cross-owner access.

## Database Rules
- Use UUID primary keys.
- Use transactions for multi-step operations.
- Use integer money values.
- Add indexes for frequently filtered columns.
- Use database constraints for duplicate prevention where possible.

## Billing Rules
- Do not create deposit bills.
- Do not support partial payments.
- Enforce one bill per `room_assignment_id + billing_month`.
- Scheduled bill generation must be idempotent.
- Manual bill generation must also be idempotent.

## Payment Rules
- Manual payment amount must equal bill amount.
- Payment gateway transaction amount must equal bill amount.
- A bill can only have one successful payment.
- Payment creation and bill update must happen in one database transaction.
- Do not mark gateway payment as paid from frontend redirect alone.
- Only verified webhook or verified server-side gateway status should mark payment as paid.

## Payment Gateway Rules
- Hide provider secret keys in environment variables.
- Never expose secret keys to frontend.
- Verify webhook signature before processing.
- Store raw webhook payload for audit.
- Make webhook processing idempotent.
- Do not create duplicate payment records if webhook is delivered multiple times.
- Store external order ID and transaction ID.
- Use provider abstraction so gateway provider can be changed later.

## Frontend Rules
- Show loading states for async actions.
- Show clear error messages.
- Prevent double submit on payment link creation.
- Tenant Pay Now button should be disabled when bill is paid or cancelled.
- For gateway pending bills, show existing checkout link if still valid.

## Security Rules
- Hash passwords.
- Use HTTPS in production.
- Use secure cookies or secure token storage.
- Rate-limit login and webhook endpoints where appropriate.
- Do not log sensitive payment data.

## Testing Rules
Required tests:
- owner cannot access another owner's room
- owner cannot access another owner's bill
- tenant cannot access another tenant's bill
- duplicate monthly bill generation does not create duplicates
- partial manual payment is rejected
- gateway webhook duplicate does not duplicate payment
- invalid gateway signature is rejected
- bill becomes paid after successful gateway webhook

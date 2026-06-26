# CLAUDE.md — Backend

## Project
Room Rental Management SaaS backend.

Stack:
- Go
- PostgreSQL
- REST JSON API
- JWT auth
- Payment gateway provider abstraction

## Required Reading Before Coding
Always read:
- /docs/product-requirements.md
- /docs/business-rules.md
- /docs/database-schema.md
- /docs/api-spec.md
- /docs/coding-rules.md
- relevant /docs/modules/*.md file

## Development Rules
- Build one module at a time.
- Do not implement unrelated modules.
- Keep controllers thin.
- Put business logic in services/use-cases.
- Validate all request input.
- Use transactions for multi-step operations.
- Use integer money values.
- Use UUID primary keys.
- Return consistent API response format.

## Multi-Tenant Security
- Every owner-owned query must filter by owner_id.
- Never accept owner_id from request body.
- Derive owner_id from authenticated owner token.
- Tenant portal queries derive tenant_id from tenant token.
- Add tests for cross-owner access.

## Billing Rules
- Monthly rent only.
- No deposit bill.
- No partial payment.
- One bill per room_assignment_id + billing_month.
- Bill generation must be idempotent.

## Payment Gateway Rules
- Never trust frontend redirect as payment success.
- Verify webhook signature.
- Store raw webhook payload.
- Process webhooks idempotently.
- Only verified successful webhook can mark bill paid.
- Payment creation and bill update must happen in one DB transaction.

## After Coding
Always provide:
- changed files
- what was implemented
- tests added/updated
- commands to run
- risks or assumptions
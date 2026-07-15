# Module 10: Manual Payment Proof Review

## Goal
Allow tenants to submit proof of an external payment and allow the owner to review, approve, or reject the submission safely.

An uploaded screenshot is treated as a payment claim, not as confirmation that the bill has been paid.

## Scope

Included:
- tenant views owner payment instructions
- tenant uploads payment proof for an unpaid or overdue bill
- tenant enters transfer information
- owner views pending payment-proof submissions
- owner approves or rejects a submission
- approved submission creates a successful manual payment
- approved submission marks the bill as paid
- first-payment activation logic
- rejected submission keeps the bill unpaid or overdue
- private payment-proof file storage
- review history and audit fields

Excluded:
- automatic bank-statement verification
- OCR-based payment verification
- partial payments
- payment allocation across multiple bills
- refunds
- automatic payment matching
- tenant approval of their own payment

## Main Tables
- bills
- payments
- manual_payment_submissions
- tenants
- rooms
- room_assignments
- owner_users

## Database Schema

Add a new table for payment-proof submissions.

```sql
CREATE TABLE manual_payment_submissions (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  bill_id UUID NOT NULL REFERENCES bills(id),
  tenant_id UUID NOT NULL REFERENCES tenants(id),

  submitted_amount INTEGER NOT NULL,
  payment_method VARCHAR(50) NOT NULL,
  transfer_date TIMESTAMP NOT NULL,
  sender_account_name VARCHAR(150),
  reference_number VARCHAR(150),

  proof_url VARCHAR(255),

  status VARCHAR(30) NOT NULL DEFAULT 'pending_review',
  tenant_notes TEXT,
  review_reason VARCHAR(80),
  review_notes TEXT,

  reviewed_by UUID REFERENCES owner_users(id),
  reviewed_at TIMESTAMP,

  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);
```

Indexes:

```sql
CREATE INDEX idx_manual_payment_submissions_owner_id
ON manual_payment_submissions(owner_id);

CREATE INDEX idx_manual_payment_submissions_bill_id
ON manual_payment_submissions(bill_id);

CREATE INDEX idx_manual_payment_submissions_tenant_id
ON manual_payment_submissions(tenant_id);

CREATE INDEX idx_manual_payment_submissions_status
ON manual_payment_submissions(status);
```

Prevent more than one unresolved submission for the same bill:

```sql
CREATE UNIQUE INDEX uq_pending_manual_submission_per_bill
ON manual_payment_submissions(bill_id)
WHERE status = 'pending_review';
```

Allowed status:

```text
pending_review
approved
rejected
cancelled
```

Suggested rejection reasons:

```text
payment_not_found
incorrect_amount
incorrect_destination
unclear_proof
duplicate_proof
wrong_billing_month
other
```

## Bill Status Behavior

Do not add a separate bill status for payment-proof review.

The bill remains:

```text
unpaid
```

or:

```text
overdue
```

while the submission has:

```text
pending_review
```

The frontend may display an additional indicator:

```text
Payment proof pending review
```

The bill only becomes `paid` after the owner approves the submission and the backend successfully creates a payment record.

## API Endpoints

### Tenant Submit Payment Proof

```http
POST /api/v1/tenant/bills/{bill_id}/manual-payment-submissions
Content-Type: multipart/form-data
```

Fields:

```text
submitted_amount
payment_method
transfer_date
sender_account_name
reference_number
notes
proof
```

Rules:
- tenant must own the bill
- bill must belong to the tenant's authenticated owner
- bill status must be unpaid or overdue
- bill must not already have a successful payment
- submitted amount must equal bill amount
- no pending submission may already exist for the bill
- proof file is required

Example response:

```json
{
  "data": {
    "id": "uuid",
    "bill_id": "uuid",
    "status": "pending_review",
    "submitted_amount": 2000000,
    "created_at": "2026-07-15T07:20:00Z"
  },
  "message": "Payment proof submitted for review"
}
```

### Tenant View Submission

```http
GET /api/v1/tenant/bills/{bill_id}/manual-payment-submission
```

Returns the current or most recent payment-proof submission for the tenant's own bill.

### Tenant Cancel Pending Submission

```http
POST /api/v1/tenant/manual-payment-submissions/{submission_id}/cancel
```

Rules:
- tenant must own the submission
- only a pending_review submission can be cancelled
- cancelling the submission does not change the bill status

### Owner List Payment-Proof Submissions

```http
GET /api/v1/owner/manual-payment-submissions?status=pending_review&tenant_id=uuid&billing_month=2026-07&page=1&limit=20
```

### Owner View Submission Detail

```http
GET /api/v1/owner/manual-payment-submissions/{submission_id}
```

The response should include:
- bill information
- tenant information
- room information
- submitted transfer details
- proof-file access URL
- submission status
- review information

### Owner Approve Submission

```http
POST /api/v1/owner/manual-payment-submissions/{submission_id}/approve
```

Request:

```json
{
  "review_notes": "Transfer verified in bank account"
}
```

### Owner Reject Submission

```http
POST /api/v1/owner/manual-payment-submissions/{submission_id}/reject
```

Request:

```json
{
  "reason": "payment_not_found",
  "review_notes": "No matching transaction was found"
}
```

## Tenant Submission Flow

```text
Tenant logs in
→ opens My Bills
→ opens an unpaid or overdue bill
→ views owner payment instructions
→ transfers the full bill amount outside the application
→ clicks Upload Payment Proof
→ enters transfer information
→ uploads screenshot
→ submits form
→ system validates bill ownership and amount
→ system stores proof in private object storage
→ system creates manual_payment_submissions record
→ submission status becomes pending_review
→ bill remains unpaid or overdue
→ tenant sees Waiting for owner verification
```

## Owner Review Flow

```text
Owner logs in
→ dashboard shows pending payment-proof count
→ owner opens Payment Reviews
→ owner selects a pending submission
→ system shows bill, tenant, room, transfer details, and screenshot
→ owner checks actual bank or e-wallet transaction history
→ owner approves or rejects the submission
```

## Approval Flow

Approval must run inside one database transaction.

```text
Owner clicks Approve Payment
→ system locks submission record
→ system locks bill record
→ system validates authenticated owner owns both records
→ system validates submission is pending_review
→ system validates bill is not paid or cancelled
→ system validates no successful payment exists
→ system validates submitted amount equals bill amount
→ system creates payments record
→ payment_source becomes manual
→ system marks bill as paid
→ system sets bill.paid_at
→ system marks submission as approved
→ system stores reviewed_by and reviewed_at
→ if this is the first bill:
   activate tenant
   activate room assignment
   mark room occupied
→ transaction commits
```

Suggested payment record:

```json
{
  "amount": 2000000,
  "payment_method": "bank_transfer",
  "payment_source": "manual",
  "reference_number": "TRX-00192",
  "payment_date": "2026-07-15T07:17:00Z"
}
```

## Rejection Flow

```text
Owner clicks Reject Proof
→ owner selects rejection reason
→ owner may enter review notes
→ system validates submission is pending_review
→ system marks submission as rejected
→ system stores reviewed_by and reviewed_at
→ bill remains unpaid or overdue
→ no payment record is created
→ tenant sees rejection reason
→ tenant may submit a new proof
```

## File Storage Rules

Store proof files in private MinIO or another S3-compatible object store.

Do not store image binary data directly in PostgreSQL.

Recommended object key:

```text
owners/{owner_id}/payment-proofs/{submission_id}/proof.{extension}
```

Store the object key in the database, not a permanent public URL.

When an authorized user views the proof:

```text
frontend requests proof access
→ backend validates owner or tenant access
→ backend creates short-lived presigned URL
→ frontend displays the image
```

File validation rules:
- bucket must be private
- allowed MIME types: image/jpeg, image/png, image/webp
- maximum file size: 5 MB
- verify file content, not only filename extension
- generate server-side filename
- do not trust the original filename
- reject SVG and executable content
- remove image metadata when practical
- do not expose internal MinIO credentials to frontend

## Business Rules

### Manual Payment Submission Rules
- A screenshot is not proof that money was received.
- Uploading proof must not create a payment record.
- Uploading proof must not mark the bill as paid.
- Only unpaid or overdue bills may receive a submission.
- Submitted amount must equal the full bill amount.
- Partial payments are rejected.
- A bill may only have one pending submission at a time.
- A rejected or cancelled submission may be followed by a new submission.
- An approved submission cannot be edited, cancelled, approved again, or rejected.

### Owner Review Rules
- Only the authenticated owner may review submissions belonging to their workspace.
- Owner must verify the transaction outside the application before approval.
- Approval creates one successful manual payment.
- Rejection does not create a payment.
- Review reason is required when rejecting.
- The reviewer and review timestamp must be stored.

### Payment Rules
- One bill can only have one successful payment.
- Payment amount must equal bill amount.
- Payment creation, bill update, submission update, and first-payment activation must happen in one transaction.
- Approval must use row locking or equivalent concurrency protection.
- Existing direct manual payment recording remains available for cash or owner-entered payments.

### Multi-Tenant Rules
- Every query must filter by authenticated owner_id.
- Never accept owner_id from the request body.
- Tenant endpoints must derive tenant_id from the tenant token.
- Tenant may only view and submit proof for their own bills.
- Owner may only review submissions from their own workspace.

## UI Pages

### Tenant Bill Detail
Show:
- bill amount
- billing month
- due date
- bill status
- owner payment instructions
- Upload Payment Proof button
- current submission status
- rejection reason when rejected

Button behavior:
- enabled for unpaid or overdue bills without a pending submission
- disabled when submission is pending_review
- hidden when bill is paid or cancelled

### Tenant Upload Payment Proof
Fields:
- bill amount, read-only
- payment method
- transfer date and time
- sender account name
- reference number, optional
- screenshot
- notes, optional

### Owner Payment Reviews
Show:
- pending review count
- tenant name
- room number
- billing month
- bill amount
- submitted amount
- transfer date
- submission date
- status

Filters:
- pending_review
- approved
- rejected
- tenant
- billing month
- submission date

### Owner Submission Detail
Show:
- tenant and room information
- bill information
- transfer information
- screenshot preview
- review history
- Approve Payment button
- Reject Proof button

## Dashboard Changes

Add:

```text
pending_payment_reviews
```

Dashboard behavior:
- count only pending_review submissions under authenticated owner
- display pending payment reviews separately from unpaid bills
- pending submissions do not count as collected revenue
- pending submissions do not count as paid bills

## Security and Reliability

Required protections:
- owner isolation on every query
- tenant isolation on every query
- private object storage
- short-lived signed proof URLs
- request size limit
- MIME and file-content validation
- rate limiting for repeated uploads
- database transaction on approval
- row-level locking during approval
- unique successful payment per bill
- unique pending submission per bill
- audit log for approve and reject actions
- do not log presigned URLs or sensitive bank data unnecessarily

## Required Tests

### Tenant Tests
- tenant can submit proof for their own unpaid bill
- tenant cannot submit proof for another tenant's bill
- tenant cannot submit proof for a paid bill
- tenant cannot submit proof for a cancelled bill
- partial submitted amount is rejected
- duplicate pending submission is rejected
- invalid file type is rejected
- oversized file is rejected

### Owner Tests
- owner can list their own submissions
- owner cannot access another owner's submission
- owner can approve pending submission
- owner can reject pending submission
- rejection reason is required
- approved submission cannot be approved twice
- rejected submission cannot be approved

### Transaction Tests
- approval creates exactly one payment
- approval marks bill paid
- approval marks submission approved
- approval stores reviewer and review timestamp
- concurrent approval requests create only one payment
- failed transaction does not partially update records
- first approved bill activates tenant, assignment, and room

## Acceptance Criteria
- Tenant can upload payment proof for their own unpaid or overdue bill.
- Uploaded proof is stored privately.
- Submission appears in the owner's pending review list.
- Uploading proof does not mark the bill as paid.
- Owner can review bill, transfer details, and screenshot.
- Owner can approve a valid submission.
- Approval creates one manual payment and marks the bill paid.
- First-payment approval activates tenant, assignment, and room.
- Owner can reject an invalid submission with a reason.
- Rejected bill remains unpaid or overdue.
- Tenant can see pending, approved, or rejected review status.
- Duplicate payments and duplicate pending submissions are prevented.
- Owner and tenant data isolation is enforced.

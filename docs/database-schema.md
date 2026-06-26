# Database Schema

## Notes
- Database examples use PostgreSQL-style types.
- Use UUID primary keys.
- Store money as integer values.
- Every owner-owned business table includes `owner_id`.
- Enforce owner isolation in every query.

## Tables Overview

```text
owners
owner_users
tenants
rooms
room_assignments
bills
payments
payment_gateway_transactions
payment_gateway_webhook_events
```

## owners
Stores owner workspace/account.

```sql
CREATE TABLE owners (
  id UUID PRIMARY KEY,
  business_name VARCHAR(150),
  full_name VARCHAR(150) NOT NULL,
  email VARCHAR(150) NOT NULL UNIQUE,
  phone_number VARCHAR(30),
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP
);
```

## owner_users
Stores login users for owner workspace. MVP can use one user per owner, but this keeps the schema ready for future staff/admin support.

```sql
CREATE TABLE owner_users (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  full_name VARCHAR(150) NOT NULL,
  email VARCHAR(150) NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'active',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP
);
```

## tenants
Stores tenant profiles and tenant portal credentials.

```sql
CREATE TABLE tenants (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  full_name VARCHAR(150) NOT NULL,
  phone_number VARCHAR(30),
  email VARCHAR(150),
  password_hash TEXT,
  identity_number VARCHAR(100),
  emergency_contact_name VARCHAR(150),
  emergency_contact_phone VARCHAR(30),
  status VARCHAR(30) NOT NULL DEFAULT 'pending_payment',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP
);

CREATE INDEX idx_tenants_owner_id ON tenants(owner_id);
CREATE INDEX idx_tenants_status ON tenants(status);
```

Allowed status:

```text
pending_payment
active
moved_out
cancelled
```

## rooms
Stores room data.

```sql
CREATE TABLE rooms (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  room_number VARCHAR(50) NOT NULL,
  room_name VARCHAR(150),
  monthly_rent INTEGER NOT NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'available',
  notes TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  UNIQUE(owner_id, room_number)
);

CREATE INDEX idx_rooms_owner_id ON rooms(owner_id);
CREATE INDEX idx_rooms_status ON rooms(status);
```

Allowed status:

```text
available
reserved
occupied
maintenance
inactive
```

## room_assignments
Connects tenant to room.

```sql
CREATE TABLE room_assignments (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  room_id UUID NOT NULL REFERENCES rooms(id),
  start_date DATE NOT NULL,
  end_date DATE,
  monthly_rent INTEGER NOT NULL,
  payment_due_day INTEGER NOT NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'pending_payment',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP
);

CREATE INDEX idx_room_assignments_owner_id ON room_assignments(owner_id);
CREATE INDEX idx_room_assignments_tenant_id ON room_assignments(tenant_id);
CREATE INDEX idx_room_assignments_room_id ON room_assignments(room_id);
CREATE INDEX idx_room_assignments_status ON room_assignments(status);
```

Allowed status:

```text
pending_payment
active
ended
cancelled
```

Recommended constraints:

```sql
-- Enforce in application or use partial indexes depending on DB support.
-- One active/pending assignment per room per owner.
-- One active/pending assignment per tenant per owner.
```

## bills
Stores monthly rent bills.

```sql
CREATE TABLE bills (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  room_id UUID NOT NULL REFERENCES rooms(id),
  room_assignment_id UUID NOT NULL REFERENCES room_assignments(id),
  bill_number VARCHAR(80) NOT NULL,
  bill_type VARCHAR(30) NOT NULL DEFAULT 'rent',
  billing_month CHAR(7) NOT NULL,
  billing_period_start DATE NOT NULL,
  billing_period_end DATE NOT NULL,
  amount INTEGER NOT NULL,
  due_date DATE NOT NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'unpaid',
  paid_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  UNIQUE(owner_id, bill_number),
  UNIQUE(room_assignment_id, billing_month)
);

CREATE INDEX idx_bills_owner_id ON bills(owner_id);
CREATE INDEX idx_bills_tenant_id ON bills(tenant_id);
CREATE INDEX idx_bills_status ON bills(status);
CREATE INDEX idx_bills_due_date ON bills(due_date);
```

Allowed status:

```text
unpaid
gateway_pending
paid
overdue
cancelled
```

## payments
Stores successful full payments.

```sql
CREATE TABLE payments (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  bill_id UUID NOT NULL REFERENCES bills(id),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  room_id UUID NOT NULL REFERENCES rooms(id),
  amount INTEGER NOT NULL,
  payment_date TIMESTAMP NOT NULL,
  payment_method VARCHAR(50) NOT NULL,
  payment_source VARCHAR(30) NOT NULL,
  gateway_transaction_id UUID REFERENCES payment_gateway_transactions(id),
  reference_number VARCHAR(150),
  notes TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE(bill_id)
);

CREATE INDEX idx_payments_owner_id ON payments(owner_id);
CREATE INDEX idx_payments_tenant_id ON payments(tenant_id);
CREATE INDEX idx_payments_payment_date ON payments(payment_date);
```

Allowed `payment_source`:

```text
manual
gateway
```

Allowed `payment_method` examples:

```text
cash
bank_transfer
e_wallet
virtual_account
credit_card
qris
other
```

## payment_gateway_transactions
Stores checkout/payment attempts created through the gateway.

```sql
CREATE TABLE payment_gateway_transactions (
  id UUID PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES owners(id),
  bill_id UUID NOT NULL REFERENCES bills(id),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  provider VARCHAR(50) NOT NULL,
  external_transaction_id VARCHAR(150),
  external_order_id VARCHAR(150) NOT NULL,
  checkout_url TEXT,
  amount INTEGER NOT NULL,
  currency VARCHAR(10) NOT NULL DEFAULT 'IDR',
  status VARCHAR(40) NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMP,
  paid_at TIMESTAMP,
  raw_create_response JSONB,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE(provider, external_order_id)
);

CREATE INDEX idx_pgt_owner_id ON payment_gateway_transactions(owner_id);
CREATE INDEX idx_pgt_bill_id ON payment_gateway_transactions(bill_id);
CREATE INDEX idx_pgt_status ON payment_gateway_transactions(status);
```

Allowed status:

```text
pending
paid
failed
expired
cancelled
```

## payment_gateway_webhook_events
Stores gateway webhook events for idempotency and audit.

```sql
CREATE TABLE payment_gateway_webhook_events (
  id UUID PRIMARY KEY,
  provider VARCHAR(50) NOT NULL,
  external_event_id VARCHAR(200),
  external_order_id VARCHAR(150),
  external_transaction_id VARCHAR(150),
  event_type VARCHAR(100),
  signature_valid BOOLEAN NOT NULL DEFAULT FALSE,
  processed BOOLEAN NOT NULL DEFAULT FALSE,
  processing_error TEXT,
  raw_payload JSONB NOT NULL,
  received_at TIMESTAMP NOT NULL,
  processed_at TIMESTAMP,
  UNIQUE(provider, external_event_id)
);

CREATE INDEX idx_pgwe_provider_order ON payment_gateway_webhook_events(provider, external_order_id);
CREATE INDEX idx_pgwe_processed ON payment_gateway_webhook_events(processed);
```

## Key Relationships

```text
owner has many rooms
owner has many tenants
room has many room_assignments
tenant has many room_assignments
room_assignment has many bills
bill has one successful payment
bill has many gateway transaction attempts
gateway transaction may create one payment
webhook events are stored for audit and idempotency
```

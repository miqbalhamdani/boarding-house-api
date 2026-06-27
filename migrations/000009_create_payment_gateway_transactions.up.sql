CREATE TABLE IF NOT EXISTS payment_gateway_transactions (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id                UUID        NOT NULL REFERENCES owners(id),
    bill_id                 UUID        NOT NULL REFERENCES bills(id),
    tenant_id               UUID        NOT NULL REFERENCES tenants(id),
    provider                VARCHAR(50) NOT NULL,
    external_transaction_id VARCHAR(150),
    external_order_id       VARCHAR(150) NOT NULL,
    checkout_url            TEXT,
    amount                  INTEGER     NOT NULL,
    currency                VARCHAR(10) NOT NULL DEFAULT 'IDR',
    status                  VARCHAR(40) NOT NULL DEFAULT 'pending',
    expires_at              TIMESTAMPTZ,
    paid_at                 TIMESTAMPTZ,
    raw_create_response     JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, external_order_id)
);

CREATE INDEX IF NOT EXISTS idx_pgt_owner_id ON payment_gateway_transactions(owner_id);
CREATE INDEX IF NOT EXISTS idx_pgt_bill_id  ON payment_gateway_transactions(bill_id);
CREATE INDEX IF NOT EXISTS idx_pgt_status   ON payment_gateway_transactions(status);

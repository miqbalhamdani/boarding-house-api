CREATE TABLE IF NOT EXISTS payments (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id               UUID        NOT NULL REFERENCES owners(id),
    bill_id                UUID        NOT NULL REFERENCES bills(id),
    tenant_id              UUID        NOT NULL REFERENCES tenants(id),
    room_id                UUID        NOT NULL REFERENCES rooms(id),
    amount                 INTEGER     NOT NULL,
    payment_date           TIMESTAMPTZ NOT NULL,
    payment_method         VARCHAR(50) NOT NULL,
    payment_source         VARCHAR(30) NOT NULL,
    gateway_transaction_id UUID        REFERENCES payment_gateway_transactions(id),
    reference_number       VARCHAR(150),
    notes                  TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- BR-030: a bill can only have one successful payment.
    UNIQUE(bill_id)
);

CREATE INDEX IF NOT EXISTS idx_payments_owner_id     ON payments(owner_id);
CREATE INDEX IF NOT EXISTS idx_payments_tenant_id    ON payments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payments_payment_date ON payments(payment_date);

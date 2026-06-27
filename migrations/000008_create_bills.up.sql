CREATE TABLE IF NOT EXISTS bills (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id             UUID        NOT NULL REFERENCES owners(id),
    tenant_id            UUID        NOT NULL REFERENCES tenants(id),
    room_id              UUID        NOT NULL REFERENCES rooms(id),
    room_assignment_id   UUID        NOT NULL REFERENCES room_assignments(id),
    bill_number          VARCHAR(80) NOT NULL,
    bill_type            VARCHAR(30) NOT NULL DEFAULT 'rent',
    billing_month        CHAR(7)     NOT NULL,
    billing_period_start DATE        NOT NULL,
    billing_period_end   DATE        NOT NULL,
    amount               INTEGER     NOT NULL,
    due_date             DATE        NOT NULL,
    status               VARCHAR(30) NOT NULL DEFAULT 'unpaid',
    paid_at              TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ,
    UNIQUE(owner_id, bill_number),
    -- BR-016: never create duplicate bills for the same assignment + billing month.
    UNIQUE(room_assignment_id, billing_month)
);

CREATE INDEX IF NOT EXISTS idx_bills_owner_id  ON bills(owner_id);
CREATE INDEX IF NOT EXISTS idx_bills_tenant_id ON bills(tenant_id);
CREATE INDEX IF NOT EXISTS idx_bills_status    ON bills(status);
CREATE INDEX IF NOT EXISTS idx_bills_due_date  ON bills(due_date);

CREATE TABLE IF NOT EXISTS room_assignments (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id        UUID        NOT NULL REFERENCES owners(id),
    tenant_id       UUID        NOT NULL REFERENCES tenants(id),
    room_id         UUID        NOT NULL REFERENCES rooms(id),
    start_date      DATE        NOT NULL,
    end_date        DATE,
    monthly_rent    INTEGER     NOT NULL,
    payment_due_day INTEGER     NOT NULL,
    status          VARCHAR(30) NOT NULL DEFAULT 'pending_payment',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_room_assignments_owner_id  ON room_assignments(owner_id);
CREATE INDEX IF NOT EXISTS idx_room_assignments_tenant_id ON room_assignments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_room_assignments_room_id   ON room_assignments(room_id);
CREATE INDEX IF NOT EXISTS idx_room_assignments_status    ON room_assignments(status);

-- BR-010: a room can only have one active or pending assignment at a time.
CREATE UNIQUE INDEX IF NOT EXISTS uq_room_assignments_active_room
    ON room_assignments (room_id)
    WHERE status IN ('pending_payment', 'active') AND deleted_at IS NULL;

-- BR-011: a tenant can only have one active or pending assignment at a time.
CREATE UNIQUE INDEX IF NOT EXISTS uq_room_assignments_active_tenant
    ON room_assignments (tenant_id)
    WHERE status IN ('pending_payment', 'active') AND deleted_at IS NULL;

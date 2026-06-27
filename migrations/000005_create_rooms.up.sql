CREATE TABLE IF NOT EXISTS rooms (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id     UUID        NOT NULL REFERENCES owners(id),
    room_number  VARCHAR(50) NOT NULL,
    room_name    VARCHAR(150),
    monthly_rent INTEGER     NOT NULL,
    status       VARCHAR(30) NOT NULL DEFAULT 'available',
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    UNIQUE(owner_id, room_number)
);

CREATE INDEX IF NOT EXISTS idx_rooms_owner_id ON rooms(owner_id);
CREATE INDEX IF NOT EXISTS idx_rooms_status   ON rooms(status);

CREATE TABLE IF NOT EXISTS tenants (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id                UUID NOT NULL REFERENCES owners(id),
    full_name               VARCHAR(150) NOT NULL,
    phone_number            VARCHAR(30),
    email                   VARCHAR(150),
    password_hash           TEXT,
    identity_number         VARCHAR(100),
    emergency_contact_name  VARCHAR(150),
    emergency_contact_phone VARCHAR(30),
    status                  VARCHAR(30)  NOT NULL DEFAULT 'pending_payment',
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tenants_owner_id ON tenants(owner_id);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);

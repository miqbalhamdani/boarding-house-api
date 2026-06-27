CREATE TABLE IF NOT EXISTS owners (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_name VARCHAR(150),
    full_name     VARCHAR(150) NOT NULL,
    email         VARCHAR(150) NOT NULL UNIQUE,
    phone_number  VARCHAR(30),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

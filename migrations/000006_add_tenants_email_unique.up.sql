-- Tenant portal login looks up tenants by email, so an email must identify at
-- most one active tenant. Enforce a case-insensitive partial unique index that
-- ignores NULL emails (tenants without portal access) and soft-deleted rows.
CREATE UNIQUE INDEX IF NOT EXISTS uq_tenants_email_active
    ON tenants (lower(email))
    WHERE email IS NOT NULL AND deleted_at IS NULL;

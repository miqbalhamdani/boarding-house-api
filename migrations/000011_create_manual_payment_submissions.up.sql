CREATE TABLE IF NOT EXISTS manual_payment_submissions (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id            UUID        NOT NULL REFERENCES owners(id),
    bill_id             UUID        NOT NULL REFERENCES bills(id),
    tenant_id           UUID        NOT NULL REFERENCES tenants(id),

    submitted_amount    INTEGER     NOT NULL,
    payment_method      VARCHAR(50) NOT NULL,
    transfer_date       TIMESTAMPTZ NOT NULL,
    sender_account_name VARCHAR(150),
    reference_number    VARCHAR(150),

    -- Object-store key for the proof file (never a public URL). NULL until the
    -- upload following row creation succeeds.
    proof_url           VARCHAR(255),

    status              VARCHAR(30) NOT NULL DEFAULT 'pending_review',
    tenant_notes        TEXT,
    review_reason       VARCHAR(80),
    review_notes        TEXT,

    reviewed_by         UUID        REFERENCES owner_users(id),
    reviewed_at         TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_manual_payment_submissions_owner_id  ON manual_payment_submissions(owner_id);
CREATE INDEX IF NOT EXISTS idx_manual_payment_submissions_bill_id   ON manual_payment_submissions(bill_id);
CREATE INDEX IF NOT EXISTS idx_manual_payment_submissions_tenant_id ON manual_payment_submissions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_manual_payment_submissions_status    ON manual_payment_submissions(status);

-- At most one unresolved (pending_review) submission per bill; rejected/
-- cancelled submissions may be followed by a new one.
CREATE UNIQUE INDEX IF NOT EXISTS uq_pending_manual_submission_per_bill
    ON manual_payment_submissions(bill_id)
    WHERE status = 'pending_review';

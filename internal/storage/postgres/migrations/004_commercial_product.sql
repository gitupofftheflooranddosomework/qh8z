CREATE TABLE IF NOT EXISTS custom_domains (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    host TEXT NOT NULL UNIQUE,
    verification_token TEXT NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS custom_domains_workspace_created_idx
    ON custom_domains (workspace_id, created_at DESC, id DESC);

ALTER TABLE links
    ADD COLUMN IF NOT EXISTS domain_id TEXT REFERENCES custom_domains(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS links_workspace_created_idx
    ON links (workspace_id, created_at DESC, slug DESC);

CREATE INDEX IF NOT EXISTS links_domain_slug_idx
    ON links (domain_id, slug)
    WHERE domain_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS workspace_billing (
    workspace_id TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    plan_code TEXT NOT NULL DEFAULT 'free' CHECK (plan_code IN ('free', 'pro')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'past_due', 'canceled')),
    provider_customer_id TEXT NOT NULL DEFAULT '',
    provider_subscription_id TEXT NOT NULL DEFAULT '',
    current_period_end TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS workspace_billing_customer_idx
    ON workspace_billing (provider_customer_id)
    WHERE provider_customer_id <> '';

CREATE UNIQUE INDEX IF NOT EXISTS workspace_billing_subscription_idx
    ON workspace_billing (provider_subscription_id)
    WHERE provider_subscription_id <> '';

CREATE TABLE IF NOT EXISTS billing_webhook_events (
    event_id TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

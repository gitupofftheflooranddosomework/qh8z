import pg from 'pg';
import { config } from './config.mjs';

const { Pool } = pg;
export const pool = new Pool({
  connectionString: config.databaseUrl,
  max: 12,
  connectionTimeoutMillis: 5_000,
  idleTimeoutMillis: 30_000,
  statement_timeout: 10_000,
  query_timeout: 12_000,
  application_name: 'qh8z-app',
});

export function translateDbError(error) {
  if (error?.constraint === 'qh8z_link_plan_limit') {
    error.status = 402;
    error.code = 'plan_limit_reached';
    error.message = 'Your active-link limit has been reached.';
  }
  return error;
}

const rawPoolQuery = pool.query.bind(pool);
pool.query = async (...args) => {
  try {
    return await rawPoolQuery(...args);
  } catch (error) {
    throw translateDbError(error);
  }
};

export async function migrate() {
  await pool.query(`
    CREATE TABLE IF NOT EXISTS users (
      id TEXT PRIMARY KEY,
      email TEXT NOT NULL UNIQUE,
      password_hash TEXT NOT NULL,
      name TEXT NOT NULL,
      plan TEXT NOT NULL DEFAULT 'free' CHECK (plan IN ('free','pro')),
      is_admin BOOLEAN NOT NULL DEFAULT FALSE,
      stripe_customer_id TEXT UNIQUE,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
    ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS terms_accepted_at TIMESTAMPTZ;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS terms_version TEXT;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS suspension_reason TEXT;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_secret_enc TEXT;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_pending_secret_enc TEXT;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_pending_created_at TIMESTAMPTZ;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_enabled_at TIMESTAMPTZ;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_recovery_hashes JSONB NOT NULL DEFAULT '[]'::jsonb;

    CREATE TABLE IF NOT EXISTS sessions (
      token_hash TEXT PRIMARY KEY,
      user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      expires_at TIMESTAMPTZ NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
    CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
    CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);

    CREATE TABLE IF NOT EXISTS auth_tokens (
      token_hash TEXT PRIMARY KEY,
      user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      purpose TEXT NOT NULL,
      expires_at TIMESTAMPTZ NOT NULL,
      used_at TIMESTAMPTZ,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
    ALTER TABLE auth_tokens DROP CONSTRAINT IF EXISTS auth_tokens_purpose_check;
    ALTER TABLE auth_tokens ADD CONSTRAINT auth_tokens_purpose_check CHECK (purpose IN ('verify_email','reset_password','mfa_login'));
    CREATE INDEX IF NOT EXISTS auth_tokens_user_purpose_idx ON auth_tokens(user_id,purpose,created_at DESC);
    CREATE INDEX IF NOT EXISTS auth_tokens_expiry_idx ON auth_tokens(expires_at);

    CREATE OR REPLACE FUNCTION qh8z_revoke_tokens_after_user_update() RETURNS TRIGGER AS $$
    BEGIN
      IF OLD.password_hash IS DISTINCT FROM NEW.password_hash THEN
        DELETE FROM auth_tokens WHERE user_id=NEW.id AND purpose='reset_password';
      END IF;
      IF OLD.email_verified_at IS DISTINCT FROM NEW.email_verified_at AND NEW.email_verified_at IS NOT NULL THEN
        DELETE FROM auth_tokens WHERE user_id=NEW.id AND purpose='verify_email';
      END IF;
      RETURN NEW;
    END;
    $$ LANGUAGE plpgsql;
    DROP TRIGGER IF EXISTS qh8z_revoke_tokens_after_user_update_trigger ON users;
    CREATE TRIGGER qh8z_revoke_tokens_after_user_update_trigger
      AFTER UPDATE OF password_hash,email_verified_at ON users
      FOR EACH ROW EXECUTE FUNCTION qh8z_revoke_tokens_after_user_update();

    CREATE TABLE IF NOT EXISTS links (
      id TEXT PRIMARY KEY,
      user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      short_code TEXT NOT NULL UNIQUE,
      long_url TEXT NOT NULL,
      title TEXT,
      custom_slug TEXT,
      shlink_domain TEXT,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      disabled_at TIMESTAMPTZ
    );
    ALTER TABLE links ADD COLUMN IF NOT EXISTS reputation_checked_at TIMESTAMPTZ;
    ALTER TABLE links ADD COLUMN IF NOT EXISTS reputation_status TEXT NOT NULL DEFAULT 'unknown';
    ALTER TABLE links ADD COLUMN IF NOT EXISTS consistency_checked_at TIMESTAMPTZ;
    ALTER TABLE links ADD COLUMN IF NOT EXISTS consistency_mismatch_at TIMESTAMPTZ;
    ALTER TABLE links ADD COLUMN IF NOT EXISTS notes TEXT;
    ALTER TABLE links ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;
    ALTER TABLE links ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
    ALTER TABLE links ADD COLUMN IF NOT EXISTS max_visits INTEGER;
    ALTER TABLE links ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
    CREATE INDEX IF NOT EXISTS links_user_id_created_idx ON links(user_id, created_at DESC);
    CREATE INDEX IF NOT EXISTS links_user_status_created_idx ON links(user_id, disabled_at, archived_at, created_at DESC);
    CREATE INDEX IF NOT EXISTS links_reputation_due_idx ON links(reputation_checked_at) WHERE disabled_at IS NULL;
    CREATE INDEX IF NOT EXISTS links_consistency_due_idx ON links(consistency_checked_at) WHERE disabled_at IS NULL;
    CREATE INDEX IF NOT EXISTS links_tags_gin_idx ON links USING GIN(tags);

    CREATE OR REPLACE FUNCTION qh8z_enforce_link_plan_limit() RETURNS TRIGGER AS $$
    DECLARE
      account_plan TEXT;
      account_limit INTEGER;
      active_count INTEGER;
    BEGIN
      IF NEW.disabled_at IS NOT NULL OR (NEW.expires_at IS NOT NULL AND NEW.expires_at <= NOW()) THEN
        RETURN NEW;
      END IF;

      PERFORM pg_advisory_xact_lock(hashtext(NEW.user_id), hashtext('qh8z-link-plan-limit'));
      SELECT plan INTO account_plan FROM users WHERE id=NEW.user_id;
      account_limit := CASE account_plan WHEN 'pro' THEN 5000 ELSE 25 END;
      SELECT COUNT(*)::int INTO active_count
        FROM links
        WHERE user_id=NEW.user_id
          AND disabled_at IS NULL
          AND (expires_at IS NULL OR expires_at > NOW())
          AND id IS DISTINCT FROM NEW.id;

      IF active_count >= account_limit THEN
        RAISE EXCEPTION 'link plan limit reached'
          USING ERRCODE='P0001', CONSTRAINT='qh8z_link_plan_limit';
      END IF;
      RETURN NEW;
    END;
    $$ LANGUAGE plpgsql;
    DROP TRIGGER IF EXISTS qh8z_enforce_link_plan_limit_trigger ON links;
    CREATE TRIGGER qh8z_enforce_link_plan_limit_trigger
      BEFORE INSERT OR UPDATE OF disabled_at,expires_at ON links
      FOR EACH ROW EXECUTE FUNCTION qh8z_enforce_link_plan_limit();

    CREATE TABLE IF NOT EXISTS shlink_create_intents (
      short_code TEXT PRIMARY KEY,
      long_url TEXT NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
    CREATE INDEX IF NOT EXISTS shlink_create_intents_created_idx ON shlink_create_intents(created_at);

    -- Once QH8Z ownership is committed, the pre-Shlink create journal has done
    -- its job and must disappear in the same database transaction. Keeping an
    -- owned intent around until a later janitor pass makes immediate
    -- disable/restore look like a concurrent create and incorrectly return 409.
    DELETE FROM shlink_create_intents intent
      USING links link
      WHERE intent.short_code=link.short_code;
    CREATE OR REPLACE FUNCTION qh8z_finalize_shlink_create_intent() RETURNS TRIGGER AS $$
    BEGIN
      DELETE FROM shlink_create_intents WHERE short_code=NEW.short_code;
      RETURN NEW;
    END;
    $$ LANGUAGE plpgsql;
    DROP TRIGGER IF EXISTS qh8z_finalize_shlink_create_intent_trigger ON links;
    CREATE TRIGGER qh8z_finalize_shlink_create_intent_trigger
      AFTER INSERT ON links
      FOR EACH ROW EXECUTE FUNCTION qh8z_finalize_shlink_create_intent();

    CREATE TABLE IF NOT EXISTS api_tokens (
      id TEXT PRIMARY KEY,
      user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      name TEXT NOT NULL,
      token_hash TEXT NOT NULL UNIQUE,
      token_prefix TEXT NOT NULL,
      scopes JSONB NOT NULL DEFAULT '["links:read","links:write"]'::jsonb,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      last_used_at TIMESTAMPTZ,
      expires_at TIMESTAMPTZ,
      revoked_at TIMESTAMPTZ
    );
    CREATE INDEX IF NOT EXISTS api_tokens_user_idx ON api_tokens(user_id, created_at DESC);
    CREATE INDEX IF NOT EXISTS api_tokens_active_hash_idx ON api_tokens(token_hash) WHERE revoked_at IS NULL;

    CREATE TABLE IF NOT EXISTS abuse_reports (
      id TEXT PRIMARY KEY,
      link_id TEXT REFERENCES links(id) ON DELETE SET NULL,
      short_code TEXT NOT NULL,
      reporter_email TEXT,
      reason TEXT NOT NULL,
      status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','reviewing','resolved','dismissed')),
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      resolved_at TIMESTAMPTZ
    );
    ALTER TABLE abuse_reports ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other';
    CREATE INDEX IF NOT EXISTS abuse_reports_status_created_idx ON abuse_reports(status, created_at DESC);

    CREATE TABLE IF NOT EXISTS audit_events (
      id BIGSERIAL PRIMARY KEY,
      actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
      event_type TEXT NOT NULL,
      target_id TEXT,
      metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
    CREATE INDEX IF NOT EXISTS audit_events_created_idx ON audit_events(created_at DESC);

    CREATE TABLE IF NOT EXISTS stripe_events (
      event_id TEXT PRIMARY KEY,
      event_type TEXT NOT NULL,
      processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
  `);
}

export async function audit(actorUserId, eventType, targetId = null, metadata = {}) {
  try {
    await pool.query(
      'INSERT INTO audit_events(actor_user_id,event_type,target_id,metadata) VALUES($1,$2,$3,$4)',
      [actorUserId, eventType, targetId, JSON.stringify(metadata)]
    );
    return true;
  } catch (error) {
    console.error(JSON.stringify({ level: 'error', event: 'audit.write_failed', auditEventType: eventType, targetId, message: error.message }));
    return false;
  }
}

export async function cleanupExpiredSessions() {
  await pool.query('DELETE FROM sessions WHERE expires_at < NOW()');
}

export async function cleanupExpiredAuthTokens() {
  await pool.query("DELETE FROM auth_tokens WHERE expires_at < NOW() OR (used_at IS NOT NULL AND used_at < NOW() - INTERVAL '1 day')");
  await pool.query("DELETE FROM api_tokens WHERE (expires_at IS NOT NULL AND expires_at < NOW() - INTERVAL '30 days') OR (revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '30 days')");
}

export async function cleanupRetainedOperationalData() {
  const requested = Number.isInteger(config.retentionDays) ? config.retentionDays : 365;
  const days = Math.min(Math.max(requested, 30), 3650);
  await pool.query(`DELETE FROM audit_events WHERE created_at < NOW() - ($1::text || ' days')::interval`, [days]);
  await pool.query(`DELETE FROM abuse_reports WHERE status IN ('resolved','dismissed') AND resolved_at < NOW() - ($1::text || ' days')::interval`, [days]);
  await pool.query(`DELETE FROM stripe_events WHERE processed_at < NOW() - ($1::text || ' days')::interval`, [days]);
}

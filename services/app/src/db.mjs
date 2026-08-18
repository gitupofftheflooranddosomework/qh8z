import pg from 'pg';
import { config } from './config.mjs';

const { Pool } = pg;
export const pool = new Pool({ connectionString: config.databaseUrl, max: 12 });

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
    CREATE INDEX IF NOT EXISTS links_user_id_created_idx ON links(user_id, created_at DESC);
    CREATE INDEX IF NOT EXISTS links_reputation_due_idx ON links(reputation_checked_at) WHERE disabled_at IS NULL;

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
    // Audit telemetry must not turn an already-completed business mutation into
    // an inconsistent 500 response. Billing uses its own transactional audit
    // helper because those rows are part of the Stripe event transaction.
    console.error(JSON.stringify({ level: 'error', event: 'audit.write_failed', auditEventType: eventType, targetId, message: error.message }));
    return false;
  }
}

export async function cleanupExpiredSessions() {
  await pool.query('DELETE FROM sessions WHERE expires_at < NOW()');
}

export async function cleanupExpiredAuthTokens() {
  await pool.query("DELETE FROM auth_tokens WHERE expires_at < NOW() OR (used_at IS NOT NULL AND used_at < NOW() - INTERVAL '1 day')");
}

export async function cleanupRetainedOperationalData() {
  const days = Math.max(config.retentionDays, 30);
  await pool.query(`DELETE FROM audit_events WHERE created_at < NOW() - ($1::text || ' days')::interval`, [days]);
  await pool.query(`DELETE FROM abuse_reports WHERE status IN ('resolved','dismissed') AND resolved_at < NOW() - ($1::text || ' days')::interval`, [days]);
  await pool.query(`DELETE FROM stripe_events WHERE processed_at < NOW() - ($1::text || ' days')::interval`, [days]);
}

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

    CREATE TABLE IF NOT EXISTS sessions (
      token_hash TEXT PRIMARY KEY,
      user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      expires_at TIMESTAMPTZ NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
    CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
    CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);

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
    CREATE INDEX IF NOT EXISTS links_user_id_created_idx ON links(user_id, created_at DESC);

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
    CREATE INDEX IF NOT EXISTS abuse_reports_status_created_idx ON abuse_reports(status, created_at DESC);

    CREATE TABLE IF NOT EXISTS audit_events (
      id BIGSERIAL PRIMARY KEY,
      actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
      event_type TEXT NOT NULL,
      target_id TEXT,
      metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );
  `);
}

export async function audit(actorUserId, eventType, targetId = null, metadata = {}) {
  await pool.query(
    'INSERT INTO audit_events(actor_user_id,event_type,target_id,metadata) VALUES($1,$2,$3,$4)',
    [actorUserId, eventType, targetId, JSON.stringify(metadata)]
  );
}

export async function cleanupExpiredSessions() {
  await pool.query('DELETE FROM sessions WHERE expires_at < NOW()');
}

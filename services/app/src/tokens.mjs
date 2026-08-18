import crypto from 'node:crypto';
import { pool } from './db.mjs';

const hash = token => crypto.createHash('sha256').update(token).digest('hex');
const RECOVERABLE_PURPOSES = new Set(['verify_email', 'reset_password']);
const STALE_CONSUMPTION_MINUTES = 2;

export async function createAuthToken(userId, purpose, ttlMinutes) {
  const token = crypto.randomBytes(32).toString('base64url');
  const client = await pool.connect();
  try {
    await client.query('BEGIN');
    // Serializes replacement for the same user/purpose so concurrent resend,
    // reset, or MFA-challenge requests cannot leave multiple current tokens.
    await client.query('SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))', [String(userId), String(purpose)]);
    await client.query('DELETE FROM auth_tokens WHERE user_id=$1 AND purpose=$2', [userId, purpose]);
    await client.query(
      "INSERT INTO auth_tokens(token_hash,user_id,purpose,expires_at) VALUES($1,$2,$3,NOW()+($4::text || ' minutes')::interval)",
      [hash(token), userId, purpose, Math.max(1, ttlMinutes)]
    );
    await client.query('COMMIT');
    return token;
  } catch (error) {
    try { await client.query('ROLLBACK'); } catch {}
    throw error;
  } finally {
    client.release();
  }
}

export async function getAuthTokenUser(token, purpose) {
  const { rows } = await pool.query(
    `SELECT user_id FROM auth_tokens
     WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at>NOW()`,
    [hash(String(token || '')), purpose]
  );
  return rows[0]?.user_id || null;
}

export async function consumeAuthToken(token, purpose) {
  const tokenHash = hash(String(token || ''));
  const client = await pool.connect();
  try {
    await client.query('BEGIN');

    // Discover the lock key without taking a row lock first. Replacement takes
    // the advisory lock before deleting rows, so taking locks in the same order
    // prevents a consume-vs-replace deadlock.
    const candidate = await client.query(
      'SELECT user_id FROM auth_tokens WHERE token_hash=$1 AND purpose=$2 AND expires_at>NOW()',
      [tokenHash, purpose]
    );
    const candidateUserId = candidate.rows[0]?.user_id;
    if (!candidateUserId) {
      await client.query('ROLLBACK');
      return null;
    }
    await client.query('SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))', [String(candidateUserId), String(purpose)]);

    const current = await client.query(
      `SELECT user_id,used_at,expires_at FROM auth_tokens
       WHERE token_hash=$1 AND purpose=$2 AND expires_at>NOW()
       FOR UPDATE`,
      [tokenHash, purpose]
    );
    const row = current.rows[0];
    if (!row || row.user_id !== candidateUserId) {
      await client.query('ROLLBACK');
      return null;
    }

    if (row.used_at) {
      const ageMs = Date.now() - new Date(row.used_at).getTime();
      const recoverable = RECOVERABLE_PURPOSES.has(purpose) && Number.isFinite(ageMs) && ageMs >= STALE_CONSUMPTION_MINUTES * 60_000;
      if (!recoverable) {
        await client.query('ROLLBACK');
        return null;
      }
      // Verification/reset mutations delete their corresponding tokens in the
      // same PostgreSQL transaction via qh8z_revoke_tokens_after_user_update.
      // A still-present, stale used token therefore means the authorized user
      // mutation never committed and it is safe to retry consumption.
      await client.query('UPDATE auth_tokens SET used_at=NULL WHERE token_hash=$1 AND purpose=$2', [tokenHash, purpose]);
    }

    const consumed = await client.query(
      `UPDATE auth_tokens SET used_at=NOW()
       WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at>NOW()
       RETURNING user_id`,
      [tokenHash, purpose]
    );
    if (!consumed.rows[0]) {
      await client.query('ROLLBACK');
      return null;
    }
    await client.query('COMMIT');
    return consumed.rows[0].user_id;
  } catch (error) {
    try { await client.query('ROLLBACK'); } catch {}
    throw error;
  } finally {
    client.release();
  }
}

export async function revokeAuthTokens(userId, purpose = null) {
  if (purpose) return pool.query('DELETE FROM auth_tokens WHERE user_id=$1 AND purpose=$2', [userId, purpose]);
  return pool.query('DELETE FROM auth_tokens WHERE user_id=$1', [userId]);
}

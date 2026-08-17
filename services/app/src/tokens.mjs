import crypto from 'node:crypto';
import { pool } from './db.mjs';

const hash = token => crypto.createHash('sha256').update(token).digest('hex');

export async function createAuthToken(userId, purpose, ttlMinutes) {
  const token = crypto.randomBytes(32).toString('base64url');
  await pool.query('DELETE FROM auth_tokens WHERE user_id=$1 AND purpose=$2 AND used_at IS NULL', [userId, purpose]);
  await pool.query(
    "INSERT INTO auth_tokens(token_hash,user_id,purpose,expires_at) VALUES($1,$2,$3,NOW()+($4::text || ' minutes')::interval)",
    [hash(token), userId, purpose, Math.max(1, ttlMinutes)]
  );
  return token;
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
  const { rows } = await pool.query(
    `UPDATE auth_tokens SET used_at=NOW()
     WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at>NOW()
     RETURNING user_id`,
    [hash(String(token || '')), purpose]
  );
  return rows[0]?.user_id || null;
}

export async function revokeAuthTokens(userId, purpose = null) {
  if (purpose) return pool.query('DELETE FROM auth_tokens WHERE user_id=$1 AND purpose=$2', [userId, purpose]);
  return pool.query('DELETE FROM auth_tokens WHERE user_id=$1', [userId]);
}

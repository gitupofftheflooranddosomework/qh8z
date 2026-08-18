import crypto from 'node:crypto';
import { pool } from './db.mjs';
import { config } from './config.mjs';

const TOKEN_PREFIX = 'qh8z_live_';
const ALLOWED_SCOPES = new Set(['links:read', 'links:write']);

const hashToken = token => crypto.createHash('sha256').update(String(token || '')).digest('hex');

export function normalizeApiScopes(scopes) {
  if (scopes === undefined || scopes === null) return ['links:read', 'links:write'];
  if (!Array.isArray(scopes)) {
    const error = new Error('API token scopes must be an array.');
    error.status = 400;
    error.code = 'invalid_api_scopes';
    throw error;
  }
  const requested = scopes.map(scope => String(scope || '').trim()).filter(Boolean);
  const invalid = requested.filter(scope => !ALLOWED_SCOPES.has(scope));
  if (invalid.length) {
    const error = new Error(`Unsupported API token scope: ${invalid[0]}`);
    error.status = 400;
    error.code = 'invalid_api_scope';
    throw error;
  }
  const normalized = [...new Set(requested)];
  if (!normalized.length) normalized.push('links:read');
  return normalized;
}

export async function createApiToken(userId, { name, scopes, expiresInDays = 365 } = {}) {
  const cleanName = String(name || 'API token').trim().slice(0, 80) || 'API token';
  const days = Math.min(Math.max(Number(expiresInDays) || 365, 1), 3650);
  const raw = `${TOKEN_PREFIX}${crypto.randomBytes(32).toString('base64url')}`;
  const tokenHash = hashToken(raw);
  const prefix = raw.slice(0, TOKEN_PREFIX.length + 8);
  const id = crypto.randomUUID();
  const normalizedScopes = normalizeApiScopes(scopes);
  const { rows } = await pool.query(
    `INSERT INTO api_tokens(id,user_id,name,token_hash,token_prefix,scopes,expires_at)
     VALUES($1,$2,$3,$4,$5,$6::jsonb,NOW()+($7::text || ' days')::interval)
     RETURNING id,name,token_prefix,scopes,created_at,last_used_at,expires_at,revoked_at`,
    [id, userId, cleanName, tokenHash, prefix, JSON.stringify(normalizedScopes), days]
  );
  return { token: raw, record: rows[0] };
}

export async function listApiTokens(userId) {
  const { rows } = await pool.query(
    `SELECT id,name,token_prefix,scopes,created_at,last_used_at,expires_at,revoked_at
     FROM api_tokens WHERE user_id=$1 ORDER BY created_at DESC`,
    [userId]
  );
  return rows;
}

export async function revokeApiToken(userId, tokenId) {
  const { rows } = await pool.query(
    `UPDATE api_tokens SET revoked_at=COALESCE(revoked_at,NOW())
     WHERE id=$1 AND user_id=$2 RETURNING id`,
    [tokenId, userId]
  );
  return Boolean(rows[0]);
}

export async function authenticateApiToken(req, res, next) {
  try {
    const authorization = String(req.headers.authorization || '');
    if (!authorization.startsWith('Bearer ')) return res.status(401).json({ error: 'api_token_required' });
    const raw = authorization.slice(7).trim();
    if (!raw.startsWith(TOKEN_PREFIX) || raw.length > 256) return res.status(401).json({ error: 'invalid_api_token' });
    const { rows } = await pool.query(
      `SELECT t.id AS api_token_id,t.scopes,t.last_used_at,u.id,u.email,u.name,u.plan,u.is_admin,u.created_at,
              u.email_verified_at,u.terms_version,u.terms_accepted_at,u.suspended_at,u.suspension_reason,u.mfa_enabled_at
       FROM api_tokens t JOIN users u ON u.id=t.user_id
       WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>NOW())`,
      [hashToken(raw)]
    );
    const user = rows[0];
    if (!user) return res.status(401).json({ error: 'invalid_api_token' });
    if (user.suspended_at) return res.status(403).json({ error: 'account_suspended' });
    if (config.emailVerificationRequired && !user.email_verified_at) return res.status(403).json({ error: 'email_verification_required' });
    if (user.terms_version !== config.termsVersion) return res.status(403).json({ error: 'terms_acceptance_required' });
    if (user.plan !== 'pro') {
      return res.status(402).json({
        error: 'feature_requires_pro',
        message: 'Developer API access requires QH8Z Pro. Existing tokens remain stored and become usable again if Pro is restored.',
      });
    }

    req.user = user;
    req.apiToken = { id: user.api_token_id, scopes: Array.isArray(user.scopes) ? user.scopes : [] };
    if (!user.last_used_at || Date.now() - new Date(user.last_used_at).getTime() > 10 * 60_000) {
      pool.query('UPDATE api_tokens SET last_used_at=NOW() WHERE id=$1', [user.api_token_id]).catch(error => {
        console.warn(JSON.stringify({ level: 'warn', event: 'api_token.last_used_failed', tokenId: user.api_token_id, message: error.message }));
      });
    }
    next();
  } catch (error) { next(error); }
}

export function requireApiScope(scope) {
  return (req, res, next) => {
    if (!req.apiToken?.scopes?.includes(scope)) return res.status(403).json({ error: 'api_scope_required', scope });
    next();
  };
}

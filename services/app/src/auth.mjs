import crypto from 'node:crypto';
import bcrypt from 'bcryptjs';
import { pool } from './db.mjs';
import { config } from './config.mjs';
import { verifyMfaUser } from './mfa.mjs';

const COOKIE = config.cookieSecure ? '__Host-qh8z_session' : 'qh8z_session';

function hashToken(token) {
  return crypto.createHash('sha256').update(token).digest('hex');
}

function parseCookies(header = '') {
  return Object.fromEntries(header.split(';').map(part => part.trim()).filter(Boolean).map(part => {
    const idx = part.indexOf('=');
    return idx === -1 ? [part, ''] : [part.slice(0, idx), decodeURIComponent(part.slice(idx + 1))];
  }));
}

export async function hashPassword(password) {
  return bcrypt.hash(password, 12);
}

export async function verifyPassword(password, hash) {
  return bcrypt.compare(password, hash);
}

export async function createSession(userId, res) {
  const token = crypto.randomBytes(32).toString('base64url');
  const tokenHash = hashToken(token);
  const role = await pool.query('SELECT is_admin FROM users WHERE id=$1', [userId]);
  const maxAgeSeconds = role.rows[0]?.is_admin ? Math.max(1, config.adminSessionHours) * 3600 : config.sessionTtlDays * 86400;
  const expires = new Date(Date.now() + maxAgeSeconds * 1000);
  await pool.query('INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)', [tokenHash, userId, expires]);
  const secure = config.cookieSecure ? '; Secure' : '';
  res.append('Set-Cookie', `${COOKIE}=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${maxAgeSeconds}${secure}`);
}

export async function destroySession(req, res) {
  const token = parseCookies(req.headers.cookie)[COOKIE];
  if (token) await pool.query('DELETE FROM sessions WHERE token_hash=$1', [hashToken(token)]);
  const secure = config.cookieSecure ? '; Secure' : '';
  res.append('Set-Cookie', `${COOKIE}=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0${secure}`);
}

export async function loadUser(req, _res, next) {
  try {
    const token = parseCookies(req.headers.cookie)[COOKIE];
    if (!token) return next();
    const { rows } = await pool.query(`
      SELECT u.id,u.email,u.name,u.plan,u.is_admin,u.stripe_customer_id,u.created_at,
             u.email_verified_at,u.terms_version,u.terms_accepted_at,u.last_login_at,
             u.suspended_at,u.suspension_reason,u.mfa_enabled_at
      FROM sessions s JOIN users u ON u.id=s.user_id
      WHERE s.token_hash=$1 AND s.expires_at > NOW()
    `, [hashToken(token)]);
    req.user = rows[0] || null;
    next();
  } catch (error) {
    next(error);
  }
}

export function hasSessionCookie(req) {
  const cookies = parseCookies(req.headers.cookie);
  return Boolean(cookies[COOKIE]);
}

export async function requireUser(req, res, next) {
  try {
    if (!req.user) return res.status(401).json({ error: 'authentication_required' });
    const route = req.originalUrl?.split('?')[0];
    if (req.user.mfa_enabled_at && req.method === 'DELETE' && route === '/api/account') {
      const { rows } = await pool.query('SELECT * FROM users WHERE id=$1', [req.user.id]);
      if (!rows[0] || !(await verifyMfaUser(rows[0], req.body?.mfaCode, false))) return res.status(401).json({ error: 'invalid_mfa_code', message: 'An authenticator or recovery code is required to delete this account.' });
    }
    if (req.user.mfa_enabled_at && req.method === 'POST' && route === '/api/account/password') {
      const { rows } = await pool.query('SELECT * FROM users WHERE id=$1', [req.user.id]);
      if (!rows[0] || !(await verifyMfaUser(rows[0], req.body?.mfaCode, true))) return res.status(401).json({ error: 'invalid_mfa_code', message: 'An authenticator or recovery code is required to change your password.' });
    }
    next();
  } catch (error) { next(error); }
}

export function requireActiveUser(req, res, next) {
  if (!req.user) return res.status(401).json({ error: 'authentication_required' });
  if (req.user.suspended_at) return res.status(403).json({ error: 'account_suspended', message: 'This account is suspended. Contact QH8Z support if you believe this is a mistake.' });
  if (config.publicLaunchMode && req.user.is_admin && req.originalUrl?.split('?')[0] === '/api/account/mfa/disable') return res.status(409).json({ error: 'admin_mfa_required', message: 'Administrator MFA cannot be disabled while public launch mode is active.' });
  next();
}

export function requireEligibleUser(req, res, next) {
  if (!req.user) return res.status(401).json({ error: 'authentication_required' });
  if (req.user.suspended_at) return res.status(403).json({ error: 'account_suspended', message: 'This account is suspended. Contact QH8Z support if you believe this is a mistake.' });
  if (config.emailVerificationRequired && !req.user.email_verified_at) return res.status(403).json({ error: 'email_verification_required', message: 'Verify your email before creating or changing short links.' });
  if (req.user.terms_version !== config.termsVersion) return res.status(403).json({ error: 'terms_acceptance_required', message: 'Accept the current QH8Z Terms before continuing.' });
  next();
}

export function requireAdmin(req, res, next) {
  if (!req.user) return res.status(401).json({ error: 'authentication_required' });
  if (!req.user.is_admin) return res.status(403).json({ error: 'admin_required' });
  if (config.publicLaunchMode && !req.user.mfa_enabled_at) return res.status(403).json({ error: 'admin_mfa_enrollment_required', message: 'Administrator MFA must be enabled before using trust and safety controls.' });
  next();
}

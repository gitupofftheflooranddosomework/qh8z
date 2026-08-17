import crypto from 'node:crypto';
import bcrypt from 'bcryptjs';
import { pool } from './db.mjs';
import { config } from './config.mjs';

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
  const expires = new Date(Date.now() + config.sessionTtlDays * 86400_000);
  await pool.query('INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)', [tokenHash, userId, expires]);
  const secure = config.cookieSecure ? '; Secure' : '';
  res.append('Set-Cookie', `${COOKIE}=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${config.sessionTtlDays * 86400}${secure}`);
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

export function requireUser(req, res, next) {
  if (!req.user) return res.status(401).json({ error: 'authentication_required' });
  next();
}

export function requireActiveUser(req, res, next) {
  if (!req.user) return res.status(401).json({ error: 'authentication_required' });
  if (req.user.suspended_at) return res.status(403).json({ error: 'account_suspended', message: 'This account is suspended. Contact QH8Z support if you believe this is a mistake.' });
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
  next();
}

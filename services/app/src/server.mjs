import crypto from 'node:crypto';
import path from 'node:path';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';
import express from 'express';
import helmet from 'helmet';
import { rateLimit } from 'express-rate-limit';
import QRCode from 'qrcode';
import { config, plans, startupProblems } from './config.mjs';
import { pool, migrate, audit, cleanupExpiredSessions, cleanupExpiredAuthTokens, cleanupRetainedOperationalData } from './db.mjs';
import { createSession, destroySession, hashPassword, verifyPassword, loadUser, requireUser, requireActiveUser, requireEligibleUser, requireAdmin, hasSessionCookie } from './auth.mjs';
import { createShortUrl, getShortUrl, editShortUrl, deleteShortUrl, getVisits, checkShlinkHealth } from './shlink.mjs';
import { billingEnabled, createCheckout, createPortal, handleStripeWebhook, cancelBillingForUser } from './billing.mjs';
import { checkUrlReputation } from './reputation.mjs';
import { verifyTurnstile } from './turnstile.mjs';
import { createAuthToken, getAuthTokenUser, consumeAuthToken, revokeAuthTokens } from './tokens.mjs';
import { mailerConfigured, mailerHealthy, sendVerificationEmail, sendPasswordResetEmail } from './mailer.mjs';
import { assertDestinationAllowed } from './destination.mjs';
import { generateMfaSetup, generateRecoveryCodes, verifyTotp, verifyMfaUser, decryptMfaSecret } from './mfa.mjs';
import { normalizeEmail, validEmail, validPassword, normalizeHttpUrl, normalizeSlug, cleanTitle, accepted, RESERVED_SLUGS } from './validation.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const publicDir = path.resolve(__dirname, '../public');
const app = express();
app.disable('x-powered-by');
app.set('trust proxy', 1);

app.use((req, res, next) => {
  req.id = String(req.headers['x-request-id'] || crypto.randomUUID()).slice(0, 128);
  res.setHeader('X-Request-Id', req.id);
  const started = Date.now();
  res.on('finish', () => {
    console.log(JSON.stringify({ level: 'info', event: 'http.request', requestId: req.id, method: req.method, path: req.path, status: res.statusCode, durationMs: Date.now() - started }));
  });
  next();
});

app.use(helmet({
  contentSecurityPolicy: {
    directives: {
      defaultSrc: ["'self'"],
      scriptSrc: ["'self'", 'https://challenges.cloudflare.com'],
      styleSrc: ["'self'", "'unsafe-inline'"],
      imgSrc: ["'self'", 'data:'],
      connectSrc: ["'self'", 'https://challenges.cloudflare.com'],
      frameSrc: ['https://challenges.cloudflare.com'],
      objectSrc: ["'none'"],
      baseUri: ["'self'"],
      formAction: ["'self'"],
      frameAncestors: ["'none'"],
    }
  },
  crossOriginEmbedderPolicy: false,
  referrerPolicy: { policy: 'no-referrer' },
}));

const loginLimiter = rateLimit({ windowMs: 15 * 60_000, limit: 12, standardHeaders: 'draft-8', legacyHeaders: false });
const signupLimiter = rateLimit({ windowMs: 60 * 60_000, limit: 8, standardHeaders: 'draft-8', legacyHeaders: false });
const recoveryLimiter = rateLimit({ windowMs: 30 * 60_000, limit: 8, standardHeaders: 'draft-8', legacyHeaders: false });
const writeLimiter = rateLimit({ windowMs: 60_000, limit: 60, standardHeaders: 'draft-8', legacyHeaders: false });
const reportLimiter = rateLimit({ windowMs: 60 * 60_000, limit: 10, standardHeaders: 'draft-8', legacyHeaders: false });

app.post('/api/billing/webhook', express.raw({ type: 'application/json', limit: '1mb' }), async (req, res, next) => {
  try {
    const type = await handleStripeWebhook(req.body, req.headers['stripe-signature']);
    res.json({ received: true, type });
  } catch (error) { next(error); }
});

app.use(express.json({ limit: '64kb' }));
app.use(express.urlencoded({ extended: false, limit: '64kb' }));
app.use(loadUser);

function safeUser(user) {
  return user ? {
    id: user.id,
    email: user.email,
    name: user.name,
    plan: user.plan,
    isAdmin: Boolean(user.is_admin),
    createdAt: user.created_at,
    emailVerified: Boolean(user.email_verified_at),
    termsVersion: user.terms_version || null,
    mustAcceptTerms: user.terms_version !== config.termsVersion,
    suspended: Boolean(user.suspended_at),
    suspensionReason: user.suspension_reason || null,
    mfaEnabled: Boolean(user.mfa_enabled_at),
  } : null;
}

function expectedOrigin(req) {
  if (config.env === 'production') return new URL(config.appBaseUrl).origin;
  const proto = req.headers['x-forwarded-proto'] || req.protocol || 'http';
  const host = req.headers['x-forwarded-host'] || req.headers.host;
  return `${proto}://${host}`;
}

function ensureSameOrigin(req, res, next) {
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(req.method) || !hasSessionCookie(req)) return next();
  const source = req.headers.origin || req.headers.referer;
  if (!source) return res.status(403).json({ error: 'origin_required' });
  try {
    if (new URL(source).origin !== expectedOrigin(req)) return res.status(403).json({ error: 'origin_rejected' });
  } catch { return res.status(403).json({ error: 'origin_rejected' }); }
  next();
}
app.use('/api', ensureSameOrigin);

function clientIp(req) {
  return req.ip || req.socket?.remoteAddress || undefined;
}

async function requireHuman(req, action) {
  return verifyTurnstile(req.body?.turnstileToken, action, clientIp(req));
}

app.get('/healthz', (_req, res) => res.json({ ok: true, service: 'qh8z' }));
app.get('/readyz', async (_req, res) => {
  const checks = { database: false, shlink: false, mailer: false, configuration: startupProblems().length === 0, adminMfa: !config.publicLaunchMode };
  try {
    await pool.query('SELECT 1');
    checks.database = true;
    if (config.publicLaunchMode) {
      const { rows } = await pool.query('SELECT COUNT(*)::int AS total, COUNT(*) FILTER (WHERE mfa_enabled_at IS NOT NULL)::int AS protected FROM users WHERE is_admin=TRUE');
      checks.adminMfa = rows[0].total > 0 && rows[0].total === rows[0].protected;
    }
  } catch {}
  try { checks.shlink = await checkShlinkHealth(); } catch {}
  try { checks.mailer = !config.emailVerificationRequired || (mailerConfigured() && await mailerHealthy()); } catch {}
  const ok = Object.values(checks).every(Boolean);
  res.status(ok ? 200 : 503).json({ ok, service: 'qh8z', publicLaunchMode: config.publicLaunchMode, checks });
});

app.get('/api/config', (_req, res) => {
  res.json({
    brand: 'QH8Z', publicShortBaseUrl: config.publicShortBaseUrl, allowSignup: config.allowSignup,
    billingEnabled: billingEnabled(), supportEmail: config.supportEmail, abuseEmail: config.abuseEmail,
    plans, turnstileSiteKey: config.turnstileSiteKey || null, turnstileRequired: config.turnstileRequired,
    emailVerificationRequired: config.emailVerificationRequired, termsVersion: config.termsVersion,
  });
});
app.get('/api/me', (req, res) => res.json({ user: safeUser(req.user) }));

app.post('/api/auth/register', signupLimiter, async (req, res, next) => {
  try {
    if (!config.allowSignup) return res.status(403).json({ error: 'signup_disabled' });
    await requireHuman(req, 'signup');
    const email = normalizeEmail(req.body.email);
    const name = String(req.body.name || '').trim().slice(0, 80);
    const password = req.body.password;
    if (!accepted(req.body.acceptTerms)) return res.status(400).json({ error: 'terms_required', message: 'Accept the QH8Z Terms to create an account.' });
    if (!name) return res.status(400).json({ error: 'name_required' });
    if (!validEmail(email)) return res.status(400).json({ error: 'invalid_email' });
    if (!validPassword(password)) return res.status(400).json({ error: 'weak_password', message: 'Use 10-72 UTF-8 bytes.' });
    const id = crypto.randomUUID();
    const passwordHash = await hashPassword(password);
    const isAdminEmail = Boolean(config.adminEmail && email === config.adminEmail);
    if (isAdminEmail) return res.status(403).json({ error: 'admin_email_reserved', message: 'This email is reserved for the locally bootstrapped QH8Z administrator.' });
    const verifiedNow = !config.emailVerificationRequired;
    const result = await pool.query(
      `INSERT INTO users(id,email,password_hash,name,is_admin,email_verified_at,terms_accepted_at,terms_version)
       VALUES($1,$2,$3,$4,FALSE,CASE WHEN $5 THEN NOW() ELSE NULL END,NOW(),$6)
       RETURNING *`,
      [id, email, passwordHash, name, verifiedNow, config.termsVersion]
    );
    await createSession(id, res);
    await audit(id, 'auth.registered', id, { termsVersion: config.termsVersion, verifiedNow });
    let verificationSent = verifiedNow;
    let verificationToken = null;
    if (!verifiedNow) {
      verificationToken = await createAuthToken(id, 'verify_email', 24 * 60);
      try { await sendVerificationEmail(result.rows[0], verificationToken); verificationSent = true; }
      catch (error) { await audit(id, 'auth.verification_email_failed', id, { message: error.message }); }
    }
    res.status(201).json({
      user: safeUser(result.rows[0]), verificationRequired: !verifiedNow, verificationSent,
      ...(config.authTokenExposeInDev && verificationToken ? { debugVerificationToken: verificationToken } : {}),
    });
  } catch (error) {
    if (error?.code === '23505') return res.status(409).json({ error: 'email_already_registered' });
    next(error);
  }
});

app.post('/api/auth/login', loginLimiter, async (req, res, next) => {
  try {
    await requireHuman(req, 'login');
    const email = normalizeEmail(req.body.email);
    const { rows } = await pool.query('SELECT * FROM users WHERE email=$1', [email]);
    const user = rows[0];
    if (!user || !(await verifyPassword(String(req.body.password || ''), user.password_hash))) return res.status(401).json({ error: 'invalid_credentials' });
    if (user.suspended_at) return res.status(403).json({ error: 'account_suspended', message: user.suspension_reason || 'This account is suspended.' });
    if (user.mfa_enabled_at) {
      const challengeToken = await createAuthToken(user.id, 'mfa_login', 5);
      await audit(user.id, 'auth.mfa_challenge', user.id);
      return res.status(202).json({ mfaRequired: true, challengeToken });
    }
    await createSession(user.id, res);
    await pool.query('UPDATE users SET last_login_at=NOW() WHERE id=$1', [user.id]);
    user.last_login_at = new Date();
    await audit(user.id, 'auth.login', user.id);
    res.json({ user: safeUser(user) });
  } catch (error) { next(error); }
});

app.post('/api/auth/mfa', loginLimiter, async (req, res, next) => {
  try {
    const challengeToken = String(req.body.challengeToken || '');
    const userId = await getAuthTokenUser(challengeToken, 'mfa_login');
    if (!userId) return res.status(400).json({ error: 'invalid_or_expired_challenge' });
    const { rows } = await pool.query('SELECT * FROM users WHERE id=$1', [userId]);
    const user = rows[0];
    if (!user || user.suspended_at) return res.status(403).json({ error: 'account_unavailable' });
    if (!(await verifyMfaUser(user, req.body.code, true))) return res.status(401).json({ error: 'invalid_mfa_code', message: 'Invalid authenticator or recovery code.' });
    const consumed = await consumeAuthToken(challengeToken, 'mfa_login');
    if (!consumed) return res.status(400).json({ error: 'invalid_or_expired_challenge' });
    await createSession(user.id, res);
    await pool.query('UPDATE users SET last_login_at=NOW() WHERE id=$1', [user.id]);
    user.last_login_at = new Date();
    await audit(user.id, 'auth.mfa_login', user.id);
    res.json({ user: safeUser(user) });
  } catch (error) { next(error); }
});

app.post('/api/auth/verify-email', recoveryLimiter, async (req, res, next) => {
  try {
    const userId = await consumeAuthToken(req.body.token, 'verify_email');
    if (!userId) return res.status(400).json({ error: 'invalid_or_expired_token', message: 'That verification link is invalid or expired.' });
    await pool.query('UPDATE users SET email_verified_at=COALESCE(email_verified_at,NOW()) WHERE id=$1', [userId]);
    await revokeAuthTokens(userId, 'verify_email');
    await audit(userId, 'auth.email_verified', userId);
    res.json({ ok: true });
  } catch (error) { next(error); }
});

app.post('/api/auth/resend-verification', requireActiveUser, recoveryLimiter, async (req, res, next) => {
  try {
    if (req.user.email_verified_at || !config.emailVerificationRequired) return res.status(204).end();
    const token = await createAuthToken(req.user.id, 'verify_email', 24 * 60);
    await sendVerificationEmail(req.user, token);
    await audit(req.user.id, 'auth.verification_resent', req.user.id);
    res.json({ ok: true, ...(config.authTokenExposeInDev ? { debugVerificationToken: token } : {}) });
  } catch (error) { next(error); }
});

app.post('/api/auth/forgot-password', recoveryLimiter, async (req, res, next) => {
  try {
    await requireHuman(req, 'forgot');
    const email = normalizeEmail(req.body.email);
    if (validEmail(email)) {
      const { rows } = await pool.query('SELECT id,email,name FROM users WHERE email=$1', [email]);
      const user = rows[0];
      if (user) {
        const token = await createAuthToken(user.id, 'reset_password', 60);
        try { await sendPasswordResetEmail(user, token); await audit(user.id, 'auth.password_reset_requested', user.id); }
        catch (error) { await audit(user.id, 'auth.password_reset_email_failed', user.id, { message: error.message }); }
        if (config.authTokenExposeInDev) return res.status(202).json({ ok: true, debugResetToken: token });
      }
    }
    res.status(202).json({ ok: true });
  } catch (error) { next(error); }
});

app.post('/api/auth/reset-password', recoveryLimiter, async (req, res, next) => {
  try {
    const password = String(req.body.password || '');
    if (!validPassword(password)) return res.status(400).json({ error: 'weak_password', message: 'Use 10-72 UTF-8 bytes.' });
    const userId = await consumeAuthToken(req.body.token, 'reset_password');
    if (!userId) return res.status(400).json({ error: 'invalid_or_expired_token', message: 'That reset link is invalid or expired.' });
    const passwordHash = await hashPassword(password);
    await pool.query('UPDATE users SET password_hash=$1,email_verified_at=COALESCE(email_verified_at,NOW()) WHERE id=$2', [passwordHash, userId]);
    await pool.query('DELETE FROM sessions WHERE user_id=$1', [userId]);
    await revokeAuthTokens(userId);
    await audit(userId, 'auth.password_reset_completed', userId);
    res.json({ ok: true });
  } catch (error) { next(error); }
});

app.post('/api/account/accept-terms', requireActiveUser, writeLimiter, async (req, res, next) => {
  try {
    if (!accepted(req.body.acceptTerms)) return res.status(400).json({ error: 'terms_required' });
    const { rows } = await pool.query('UPDATE users SET terms_version=$1,terms_accepted_at=NOW() WHERE id=$2 RETURNING *', [config.termsVersion, req.user.id]);
    await audit(req.user.id, 'account.terms_accepted', req.user.id, { termsVersion: config.termsVersion });
    res.json({ user: safeUser(rows[0]) });
  } catch (error) { next(error); }
});

app.post('/api/account/mfa/setup', requireActiveUser, loginLimiter, async (req, res, next) => {
  try {
    const password = String(req.body.password || '');
    const { rows } = await pool.query('SELECT password_hash,email,mfa_enabled_at FROM users WHERE id=$1', [req.user.id]);
    if (!rows[0] || !(await verifyPassword(password, rows[0].password_hash))) return res.status(401).json({ error: 'invalid_password' });
    if (rows[0].mfa_enabled_at) return res.status(409).json({ error: 'mfa_already_enabled', message: 'Disable existing MFA before replacing it.' });
    const setup = generateMfaSetup(rows[0].email);
    await pool.query('UPDATE users SET mfa_pending_secret_enc=$1,mfa_pending_created_at=NOW() WHERE id=$2', [setup.encryptedSecret, req.user.id]);
    await audit(req.user.id, 'account.mfa_setup_started', req.user.id);
    res.json({ secret: setup.secret, otpauthUri: setup.otpauthUri, qrDataUrl: await QRCode.toDataURL(setup.otpauthUri, { margin: 1, errorCorrectionLevel: 'M' }) });
  } catch (error) { next(error); }
});

app.post('/api/account/mfa/confirm', requireActiveUser, loginLimiter, async (req, res, next) => {
  try {
    const { rows } = await pool.query('SELECT mfa_pending_secret_enc,mfa_pending_created_at FROM users WHERE id=$1', [req.user.id]);
    const pending = rows[0];
    if (!pending?.mfa_pending_secret_enc || !pending.mfa_pending_created_at || Date.now() - new Date(pending.mfa_pending_created_at).getTime() > 15 * 60_000) {
      return res.status(400).json({ error: 'mfa_setup_expired', message: 'Start MFA setup again.' });
    }
    const secret = decryptMfaSecret(pending.mfa_pending_secret_enc);
    if (!verifyTotp(secret, req.body.code)) return res.status(400).json({ error: 'invalid_mfa_code' });
    const recovery = generateRecoveryCodes();
    await pool.query(`UPDATE users SET mfa_secret_enc=mfa_pending_secret_enc,mfa_enabled_at=NOW(),mfa_recovery_hashes=$1::jsonb,mfa_pending_secret_enc=NULL,mfa_pending_created_at=NULL WHERE id=$2`, [JSON.stringify(recovery.hashes), req.user.id]);
    await audit(req.user.id, 'account.mfa_enabled', req.user.id);
    res.json({ ok: true, recoveryCodes: recovery.codes });
  } catch (error) { next(error); }
});

app.post('/api/account/mfa/disable', requireActiveUser, loginLimiter, async (req, res, next) => {
  try {
    const { rows } = await pool.query('SELECT * FROM users WHERE id=$1', [req.user.id]);
    const user = rows[0];
    if (!user?.mfa_enabled_at) return res.status(204).end();
    if (!(await verifyPassword(String(req.body.password || ''), user.password_hash))) return res.status(401).json({ error: 'invalid_password' });
    if (!(await verifyMfaUser(user, req.body.code, true))) return res.status(401).json({ error: 'invalid_mfa_code' });
    await pool.query("UPDATE users SET mfa_secret_enc=NULL,mfa_enabled_at=NULL,mfa_recovery_hashes='[]'::jsonb,mfa_pending_secret_enc=NULL,mfa_pending_created_at=NULL WHERE id=$1", [req.user.id]);
    await audit(req.user.id, 'account.mfa_disabled', req.user.id);
    res.status(204).end();
  } catch (error) { next(error); }
});

app.post('/api/account/password', requireUser, loginLimiter, async (req, res, next) => {
  try {
    const currentPassword = String(req.body.currentPassword || '');
    const newPassword = String(req.body.newPassword || '');
    if (!validPassword(newPassword)) return res.status(400).json({ error: 'weak_password', message: 'Use 10-72 UTF-8 bytes.' });
    const { rows } = await pool.query('SELECT password_hash FROM users WHERE id=$1', [req.user.id]);
    if (!rows[0] || !(await verifyPassword(currentPassword, rows[0].password_hash))) return res.status(401).json({ error: 'invalid_current_password' });
    const passwordHash = await hashPassword(newPassword);
    await pool.query('UPDATE users SET password_hash=$1 WHERE id=$2', [passwordHash, req.user.id]);
    await pool.query('DELETE FROM sessions WHERE user_id=$1', [req.user.id]);
    await createSession(req.user.id, res);
    await revokeAuthTokens(req.user.id, 'reset_password');
    await audit(req.user.id, 'account.password_changed', req.user.id);
    res.json({ ok: true });
  } catch (error) { next(error); }
});

app.get('/api/account/export', requireUser, async (req, res, next) => {
  try {
    const links = await pool.query('SELECT short_code,long_url,title,custom_slug,created_at,updated_at,disabled_at FROM links WHERE user_id=$1 ORDER BY created_at DESC', [req.user.id]);
    res.setHeader('Content-Disposition', 'attachment; filename="qh8z-export.json"');
    res.json({ exportedAt: new Date().toISOString(), account: safeUser(req.user), links: links.rows });
  } catch (error) { next(error); }
});

app.delete('/api/account', requireUser, loginLimiter, async (req, res, next) => {
  try {
    const password = String(req.body.password || '');
    const userRow = await pool.query('SELECT * FROM users WHERE id=$1', [req.user.id]);
    if (!userRow.rows[0] || !(await verifyPassword(password, userRow.rows[0].password_hash))) return res.status(401).json({ error: 'invalid_password' });
    const links = await pool.query('SELECT short_code FROM links WHERE user_id=$1 AND disabled_at IS NULL', [req.user.id]);
    for (const link of links.rows) {
      try { await deleteShortUrl(link.short_code); } catch (error) { if (error?.status !== 404) throw error; }
    }
    await cancelBillingForUser(userRow.rows[0]);
    const userId = req.user.id;
    await audit(userId, 'account.deleted', userId, { activeLinksRemoved: links.rows.length });
    await destroySession(req, res);
    await pool.query('DELETE FROM users WHERE id=$1', [userId]);
    res.status(204).end();
  } catch (error) { next(error); }
});

app.post('/api/auth/logout', requireUser, async (req, res, next) => {
  try {
    const userId = req.user.id;
    await destroySession(req, res);
    await audit(userId, 'auth.logout', userId);
    res.status(204).end();
  } catch (error) { next(error); }
});

async function loadOwnedLink(req, res, next) {
  try {
    const { rows } = await pool.query('SELECT * FROM links WHERE id=$1 AND user_id=$2', [req.params.id, req.user.id]);
    if (!rows[0]) return res.status(404).json({ error: 'link_not_found' });
    req.link = rows[0]; next();
  } catch (error) { next(error); }
}

app.get('/api/links', requireUser, async (req, res, next) => {
  try {
    const limit = Math.min(Math.max(Number(req.query.limit) || 50, 1), 100);
    const { rows } = await pool.query('SELECT id,short_code,long_url,title,custom_slug,created_at,updated_at,disabled_at FROM links WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2', [req.user.id, limit]);
    res.json({ links: rows.map(row => ({ ...row, short_url: `${config.publicShortBaseUrl}/${row.short_code}` })) });
  } catch (error) { next(error); }
});

async function createNonReservedShortUrl(payload) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    const upstream = await createShortUrl(payload);
    if (!RESERVED_SLUGS.has(String(upstream.shortCode || '').toLowerCase())) return upstream;
    try { await deleteShortUrl(upstream.shortCode); } catch {}
    if (payload.customSlug) throw Object.assign(new Error('That alias is reserved by QH8Z'), { status: 409 });
  }
  throw Object.assign(new Error('Could not generate a safe short code'), { status: 503 });
}

async function validateDestination(longUrl, userId, targetId = null) {
  assertDestinationAllowed(longUrl);
  const reputation = await checkUrlReputation(longUrl);
  if (reputation.threats.length) {
    await audit(userId, 'link.blocked_unsafe', targetId, { hostname: new URL(longUrl).hostname, threats: reputation.threats });
    const error = new Error('That destination is flagged as unsafe.');
    error.status = 422;
    error.code = 'unsafe_destination';
    error.threats = reputation.threats;
    throw error;
  }
}

app.post('/api/links', requireEligibleUser, writeLimiter, async (req, res, next) => {
  try {
    const plan = plans[req.user.plan] || plans.free;
    const countResult = await pool.query('SELECT COUNT(*)::int AS count FROM links WHERE user_id=$1 AND disabled_at IS NULL', [req.user.id]);
    if (countResult.rows[0].count >= plan.links) return res.status(402).json({ error: 'plan_limit_reached', limit: plan.links, plan: req.user.plan });
    let longUrl; let customSlug;
    try { longUrl = normalizeHttpUrl(req.body.longUrl); customSlug = normalizeSlug(req.body.customSlug); assertDestinationAllowed(longUrl); }
    catch (error) { return res.status(400).json({ error: 'invalid_link', message: error.message }); }
    const title = cleanTitle(req.body.title);
    await validateDestination(longUrl, req.user.id);
    const upstream = await createNonReservedShortUrl({ longUrl, customSlug, title });
    const shortCode = upstream.shortCode;
    if (!shortCode) throw new Error('Shlink did not return a shortCode');
    const id = crypto.randomUUID();
    try {
      const result = await pool.query('INSERT INTO links(id,user_id,short_code,long_url,title,custom_slug,shlink_domain) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *', [id, req.user.id, shortCode, longUrl, title, customSlug, upstream.domain || null]);
      await audit(req.user.id, 'link.created', id, { shortCode });
      res.status(201).json({ link: { ...result.rows[0], short_url: `${config.publicShortBaseUrl}/${shortCode}`, visits: upstream.visitsSummary || { total: 0, nonBots: 0, bots: 0 } } });
    } catch (dbError) {
      try { await deleteShortUrl(shortCode); } catch {}
      throw dbError;
    }
  } catch (error) {
    if (error?.code === 'unsafe_destination') return res.status(422).json({ error: error.code, message: error.message, threats: error.threats });
    if (error?.status === 400 || error?.status === 409) return res.status(409).json({ error: 'short_url_rejected', message: error.message });
    next(error);
  }
});

app.get('/api/links/:id/stats', requireUser, loadOwnedLink, async (req, res, next) => {
  try {
    const upstream = await getShortUrl(req.link.short_code);
    res.json({ shortCode: req.link.short_code, shortUrl: `${config.publicShortBaseUrl}/${req.link.short_code}`, visits: upstream.visitsSummary || { total: 0, nonBots: 0, bots: 0 }, title: upstream.title || req.link.title, longUrl: upstream.longUrl || req.link.long_url });
  } catch (error) { next(error); }
});
app.get('/api/links/:id/visits', requireUser, loadOwnedLink, async (req, res, next) => {
  try { res.json(await getVisits(req.link.short_code, Math.max(Number(req.query.page) || 1, 1), 50)); }
  catch (error) { next(error); }
});
app.get('/api/links/:id/qr.svg', requireUser, loadOwnedLink, async (req, res, next) => {
  try { res.type('image/svg+xml').send(await QRCode.toString(`${config.publicShortBaseUrl}/${req.link.short_code}`, { type: 'svg', margin: 1, errorCorrectionLevel: 'M' })); }
  catch (error) { next(error); }
});
app.patch('/api/links/:id', requireEligibleUser, writeLimiter, loadOwnedLink, async (req, res, next) => {
  try {
    let longUrl;
    try { longUrl = normalizeHttpUrl(req.body.longUrl ?? req.link.long_url); assertDestinationAllowed(longUrl); }
    catch (error) { return res.status(400).json({ error: 'invalid_link', message: error.message }); }
    const title = req.body.title === undefined ? req.link.title : cleanTitle(req.body.title);
    await validateDestination(longUrl, req.user.id, req.link.id);
    await editShortUrl(req.link.short_code, { longUrl, title });
    const { rows } = await pool.query('UPDATE links SET long_url=$1,title=$2,updated_at=NOW() WHERE id=$3 RETURNING *', [longUrl, title, req.link.id]);
    await audit(req.user.id, 'link.updated', req.link.id, { shortCode: req.link.short_code });
    res.json({ link: { ...rows[0], short_url: `${config.publicShortBaseUrl}/${req.link.short_code}` } });
  } catch (error) {
    if (error?.code === 'unsafe_destination') return res.status(422).json({ error: error.code, message: error.message, threats: error.threats });
    next(error);
  }
});
app.delete('/api/links/:id', requireEligibleUser, writeLimiter, loadOwnedLink, async (req, res, next) => {
  try {
    if (!req.link.disabled_at) {
      await deleteShortUrl(req.link.short_code);
      await pool.query('UPDATE links SET disabled_at=NOW(),updated_at=NOW() WHERE id=$1', [req.link.id]);
      await audit(req.user.id, 'link.disabled', req.link.id, { shortCode: req.link.short_code });
    }
    res.status(204).end();
  } catch (error) {
    if (error?.status === 404) { await pool.query('UPDATE links SET disabled_at=COALESCE(disabled_at,NOW()),updated_at=NOW() WHERE id=$1', [req.link.id]); return res.status(204).end(); }
    next(error);
  }
});

app.post('/api/report', reportLimiter, async (req, res, next) => {
  try {
    await requireHuman(req, 'report');
    const shortCode = String(req.body.shortCode || '').trim().slice(0, 128);
    const reason = String(req.body.reason || '').trim().slice(0, 2000);
    const category = ['phishing','malware','spam','fraud','illegal','harassment','other'].includes(String(req.body.category || 'other')) ? String(req.body.category || 'other') : 'other';
    const reporterEmail = normalizeEmail(req.body.email || '') || null;
    if (!shortCode || !reason) return res.status(400).json({ error: 'short_code_and_reason_required' });
    if (reporterEmail && !validEmail(reporterEmail)) return res.status(400).json({ error: 'invalid_email' });
    const linkResult = await pool.query('SELECT id FROM links WHERE short_code=$1 LIMIT 1', [shortCode]);
    const id = crypto.randomUUID();
    await pool.query('INSERT INTO abuse_reports(id,link_id,short_code,reporter_email,reason,category) VALUES($1,$2,$3,$4,$5,$6)', [id, linkResult.rows[0]?.id || null, shortCode, reporterEmail, reason, category]);
    await audit(req.user?.id || null, 'abuse.reported', id, { shortCode, category });
    res.status(201).json({ ok: true, reportId: id });
  } catch (error) { next(error); }
});

app.get('/api/admin/reports', requireAdmin, async (_req, res, next) => {
  try {
    const { rows } = await pool.query("SELECT r.*,l.long_url,l.user_id FROM abuse_reports r LEFT JOIN links l ON l.id=r.link_id ORDER BY CASE WHEN r.status='open' THEN 0 ELSE 1 END, r.created_at DESC LIMIT 200");
    res.json({ reports: rows });
  } catch (error) { next(error); }
});
app.patch('/api/admin/reports/:id', requireAdmin, writeLimiter, async (req, res, next) => {
  try {
    const status = String(req.body.status || '');
    if (!['open','reviewing','resolved','dismissed'].includes(status)) return res.status(400).json({ error: 'invalid_status' });
    const { rows } = await pool.query("UPDATE abuse_reports SET status=$1,resolved_at=CASE WHEN $1 IN ('resolved','dismissed') THEN NOW() ELSE NULL END WHERE id=$2 RETURNING *", [status, req.params.id]);
    if (!rows[0]) return res.status(404).json({ error: 'report_not_found' });
    await audit(req.user.id, 'abuse.status_changed', req.params.id, { status });
    res.json({ report: rows[0] });
  } catch (error) { next(error); }
});
app.post('/api/admin/links/:id/disable', requireAdmin, writeLimiter, async (req, res, next) => {
  try {
    const { rows } = await pool.query('SELECT * FROM links WHERE id=$1', [req.params.id]);
    const link = rows[0];
    if (!link) return res.status(404).json({ error: 'link_not_found' });
    try { await deleteShortUrl(link.short_code); } catch (error) { if (error?.status !== 404) throw error; }
    await pool.query('UPDATE links SET disabled_at=COALESCE(disabled_at,NOW()),updated_at=NOW() WHERE id=$1', [link.id]);
    await audit(req.user.id, 'admin.link_disabled', link.id, { shortCode: link.short_code });
    res.json({ ok: true });
  } catch (error) { next(error); }
});
app.get('/api/admin/users', requireAdmin, async (req, res, next) => {
  try {
    const q = String(req.query.q || '').trim().slice(0, 120);
    const params = [];
    let where = '';
    if (q) { params.push(`%${q}%`); where = 'WHERE email ILIKE $1 OR name ILIKE $1'; }
    const { rows } = await pool.query(`SELECT id,email,name,plan,is_admin,email_verified_at,created_at,last_login_at,suspended_at,suspension_reason,mfa_enabled_at FROM users ${where} ORDER BY created_at DESC LIMIT 100`, params);
    res.json({ users: rows });
  } catch (error) { next(error); }
});
app.post('/api/admin/users/:id/suspend', requireAdmin, writeLimiter, async (req, res, next) => {
  try {
    if (req.params.id === req.user.id) return res.status(400).json({ error: 'cannot_suspend_self' });
    const reason = String(req.body.reason || 'Abuse or policy enforcement').trim().slice(0, 500);
    const { rows } = await pool.query('SELECT * FROM users WHERE id=$1', [req.params.id]);
    const user = rows[0];
    if (!user) return res.status(404).json({ error: 'user_not_found' });
    if (user.is_admin) return res.status(400).json({ error: 'cannot_suspend_admin' });
    const links = await pool.query('SELECT id,short_code FROM links WHERE user_id=$1 AND disabled_at IS NULL', [user.id]);
    for (const link of links.rows) {
      try { await deleteShortUrl(link.short_code); } catch (error) { if (error?.status !== 404) throw error; }
    }
    await pool.query('UPDATE links SET disabled_at=COALESCE(disabled_at,NOW()),updated_at=NOW() WHERE user_id=$1 AND disabled_at IS NULL', [user.id]);
    await pool.query('UPDATE users SET suspended_at=COALESCE(suspended_at,NOW()),suspension_reason=$1 WHERE id=$2', [reason, user.id]);
    await pool.query('DELETE FROM sessions WHERE user_id=$1', [user.id]);
    await audit(req.user.id, 'admin.user_suspended', user.id, { reason, linksDisabled: links.rows.length });
    res.json({ ok: true, linksDisabled: links.rows.length });
  } catch (error) { next(error); }
});
app.post('/api/admin/users/:id/unsuspend', requireAdmin, writeLimiter, async (req, res, next) => {
  try {
    const { rows } = await pool.query('UPDATE users SET suspended_at=NULL,suspension_reason=NULL WHERE id=$1 RETURNING id', [req.params.id]);
    if (!rows[0]) return res.status(404).json({ error: 'user_not_found' });
    await audit(req.user.id, 'admin.user_unsuspended', req.params.id);
    res.json({ ok: true, note: 'Previously disabled links remain disabled.' });
  } catch (error) { next(error); }
});

app.post('/api/billing/checkout', requireEligibleUser, writeLimiter, async (req, res, next) => { try { const session = await createCheckout(req.user); res.json({ url: session.url }); } catch (error) { next(error); } });
app.post('/api/billing/portal', requireUser, writeLimiter, async (req, res, next) => { try { const session = await createPortal(req.user); res.json({ url: session.url }); } catch (error) { next(error); } });

function sendLegalTemplate(file, res) {
  const template = fs.readFileSync(path.join(publicDir, file), 'utf8');
  const html = template
    .replaceAll('{{LEGAL_OPERATOR_NAME}}', config.legalOperatorName || 'the QH8Z operator')
    .replaceAll('{{LEGAL_JURISDICTION}}', config.legalJurisdiction || 'the operator jurisdiction')
    .replaceAll('{{SUPPORT_EMAIL}}', config.supportEmail)
    .replaceAll('{{ABUSE_EMAIL}}', config.abuseEmail);
  res.type('html').send(html);
}

app.use('/assets', express.static(publicDir, { maxAge: config.env === 'production' ? '1h' : 0, index: false }));
app.get('/favicon.svg', (_req, res) => res.sendFile(path.join(publicDir, 'favicon.svg')));
app.get('/robots.txt', (_req, res) => res.sendFile(path.join(publicDir, 'robots.txt')));
app.get('/.well-known/security.txt', (_req, res) => res.type('text/plain').sendFile(path.join(publicDir, '.well-known/security.txt')));
app.get('/app', (_req, res) => res.sendFile(path.join(publicDir, 'app.html')));
app.get('/login', (_req, res) => res.redirect(302, '/app#login'));
app.get('/signup', (_req, res) => res.redirect(302, '/app#signup'));
app.get('/verify', (_req, res) => res.sendFile(path.join(publicDir, 'verify.html')));
app.get('/forgot', (_req, res) => res.sendFile(path.join(publicDir, 'forgot.html')));
app.get('/reset', (_req, res) => res.sendFile(path.join(publicDir, 'reset.html')));
app.get('/report', (_req, res) => res.sendFile(path.join(publicDir, 'report.html')));
app.get('/privacy', (_req, res) => sendLegalTemplate('privacy.html', res));
app.get('/terms', (_req, res) => sendLegalTemplate('terms.html', res));
app.get('/security', (_req, res) => res.sendFile(path.join(publicDir, 'security.html')));
app.get('/', (_req, res) => res.sendFile(path.join(publicDir, 'index.html')));
app.use((req, res) => { if (req.path.startsWith('/api/')) return res.status(404).json({ error: 'not_found' }); res.status(404).sendFile(path.join(publicDir, '404.html')); });
app.use((error, req, res, _next) => {
  console.error(JSON.stringify({ level: 'error', event: 'http.error', requestId: req.id, message: error?.message, stack: config.env === 'production' ? undefined : error?.stack }));
  const status = Number(error?.status) >= 400 && Number(error?.status) < 600 ? Number(error.status) : 500;
  res.status(status).json({ error: error?.code || (status === 500 ? 'internal_error' : 'request_failed'), message: status < 500 ? error.message : 'Something went wrong', requestId: req.id });
});

await migrate();
await cleanupExpiredSessions();
await cleanupExpiredAuthTokens();
await cleanupRetainedOperationalData();
const problems = startupProblems();
const adminCount = await pool.query('SELECT COUNT(*)::int AS count FROM users WHERE is_admin=TRUE');
if (adminCount.rows[0].count === 0 && config.publicLaunchMode && (!config.adminEmail || !config.adminBootstrapSecret)) problems.push('No administrator exists; ADMIN_EMAIL and ADMIN_BOOTSTRAP_SECRET are required for first public launch');
if (config.adminEmail) await pool.query('UPDATE users SET email_verified_at=COALESCE(email_verified_at,NOW()) WHERE email=$1 AND is_admin=TRUE', [config.adminEmail]);
if (problems.length) throw new Error(`QH8Z startup blocked: ${problems.join('; ')}`);
setInterval(() => Promise.all([cleanupExpiredSessions(), cleanupExpiredAuthTokens(), cleanupRetainedOperationalData()]).catch(console.error), 6 * 60 * 60_000).unref();
app.listen(config.port, '0.0.0.0', () => console.log(JSON.stringify({ level: 'info', event: 'app.started', port: config.port, env: config.env, publicLaunchMode: config.publicLaunchMode })));

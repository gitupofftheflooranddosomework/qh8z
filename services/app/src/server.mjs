import crypto from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import express from 'express';
import helmet from 'helmet';
import { rateLimit } from 'express-rate-limit';
import QRCode from 'qrcode';
import { config, plans } from './config.mjs';
import { pool, migrate, audit, cleanupExpiredSessions } from './db.mjs';
import { createSession, destroySession, hashPassword, verifyPassword, loadUser, requireUser, requireAdmin } from './auth.mjs';
import { createShortUrl, getShortUrl, editShortUrl, deleteShortUrl, getVisits } from './shlink.mjs';
import { billingEnabled, createCheckout, createPortal, handleStripeWebhook } from './billing.mjs';
import { checkUrlReputation } from './reputation.mjs';
import { normalizeEmail, validEmail, validPassword, normalizeHttpUrl, normalizeSlug, cleanTitle } from './validation.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const publicDir = path.resolve(__dirname, '../public');
const app = express();
app.disable('x-powered-by');
app.set('trust proxy', 1);

app.use(helmet({
  contentSecurityPolicy: {
    directives: {
      defaultSrc: ["'self'"],
      scriptSrc: ["'self'"],
      styleSrc: ["'self'", "'unsafe-inline'"],
      imgSrc: ["'self'", 'data:'],
      connectSrc: ["'self'"],
      frameSrc: ["'none'"],
      objectSrc: ["'none'"],
      baseUri: ["'self'"],
      formAction: ["'self'"]
    }
  },
  crossOriginEmbedderPolicy: false
}));

const authLimiter = rateLimit({ windowMs: 15 * 60_000, limit: 30, standardHeaders: 'draft-8', legacyHeaders: false });
const writeLimiter = rateLimit({ windowMs: 60_000, limit: 60, standardHeaders: 'draft-8', legacyHeaders: false });
const reportLimiter = rateLimit({ windowMs: 60 * 60_000, limit: 20, standardHeaders: 'draft-8', legacyHeaders: false });

// Stripe requires the exact raw request body for signature verification.
app.post('/api/billing/webhook', express.raw({ type: 'application/json', limit: '1mb' }), async (req, res, next) => {
  try {
    const type = await handleStripeWebhook(req.body, req.headers['stripe-signature']);
    res.json({ received: true, type });
  } catch (error) {
    next(error);
  }
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
    createdAt: user.created_at
  } : null;
}

function ensureSameOrigin(req, res, next) {
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(req.method)) return next();
  const origin = req.headers.origin;
  if (!origin) return next(); // API clients are allowed; cookie-auth browsers send Origin.
  try {
    const host = req.headers['x-forwarded-host'] || req.headers.host;
    if (new URL(origin).host !== host) return res.status(403).json({ error: 'origin_rejected' });
  } catch {
    return res.status(403).json({ error: 'origin_rejected' });
  }
  next();
}
app.use('/api', ensureSameOrigin);

app.get('/healthz', async (_req, res) => {
  try {
    await pool.query('SELECT 1');
    res.json({ ok: true, service: 'qh8z', shlinkConfigured: Boolean(config.shlinkApiKey), billingEnabled: billingEnabled(), urlReputationConfigured: Boolean(config.webRiskApiKey), urlReputationRequired: config.webRiskRequired });
  } catch {
    res.status(503).json({ ok: false, service: 'qh8z' });
  }
});

app.get('/api/config', (_req, res) => {
  res.json({
    brand: 'QH8Z',
    publicShortBaseUrl: config.publicShortBaseUrl,
    allowSignup: config.allowSignup,
    billingEnabled: billingEnabled(),
    supportEmail: config.supportEmail,
    plans
  });
});

app.get('/api/me', (req, res) => res.json({ user: safeUser(req.user) }));

app.post('/api/auth/register', authLimiter, async (req, res, next) => {
  try {
    if (!config.allowSignup) return res.status(403).json({ error: 'signup_disabled' });
    const email = normalizeEmail(req.body.email);
    const name = String(req.body.name || '').trim().slice(0, 80);
    const password = req.body.password;
    if (!name) return res.status(400).json({ error: 'name_required' });
    if (!validEmail(email)) return res.status(400).json({ error: 'invalid_email' });
    if (!validPassword(password)) return res.status(400).json({ error: 'weak_password', message: 'Use at least 10 characters.' });

    const id = crypto.randomUUID();
    const passwordHash = await hashPassword(password);
    const isAdmin = Boolean(config.adminEmail && email === config.adminEmail);
    const result = await pool.query(
      'INSERT INTO users(id,email,password_hash,name,is_admin) VALUES($1,$2,$3,$4,$5) RETURNING id,email,name,plan,is_admin,created_at',
      [id, email, passwordHash, name, isAdmin]
    );
    await createSession(id, res);
    await audit(id, 'auth.registered', id);
    res.status(201).json({ user: safeUser(result.rows[0]) });
  } catch (error) {
    if (error?.code === '23505') return res.status(409).json({ error: 'email_already_registered' });
    next(error);
  }
});

app.post('/api/auth/login', authLimiter, async (req, res, next) => {
  try {
    const email = normalizeEmail(req.body.email);
    const { rows } = await pool.query('SELECT * FROM users WHERE email=$1', [email]);
    const user = rows[0];
    if (!user || !(await verifyPassword(String(req.body.password || ''), user.password_hash))) {
      return res.status(401).json({ error: 'invalid_credentials' });
    }
    await createSession(user.id, res);
    await audit(user.id, 'auth.login', user.id);
    res.json({ user: safeUser(user) });
  } catch (error) {
    next(error);
  }
});

app.post('/api/account/password', requireUser, authLimiter, async (req, res, next) => {
  try {
    const currentPassword = String(req.body.currentPassword || '');
    const newPassword = String(req.body.newPassword || '');
    if (!validPassword(newPassword)) return res.status(400).json({ error: 'weak_password', message: 'Use at least 10 characters.' });
    const { rows } = await pool.query('SELECT password_hash FROM users WHERE id=$1', [req.user.id]);
    if (!rows[0] || !(await verifyPassword(currentPassword, rows[0].password_hash))) return res.status(401).json({ error: 'invalid_current_password' });
    const passwordHash = await hashPassword(newPassword);
    await pool.query('UPDATE users SET password_hash=$1 WHERE id=$2', [passwordHash, req.user.id]);
    await pool.query('DELETE FROM sessions WHERE user_id=$1', [req.user.id]);
    await createSession(req.user.id, res);
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

app.delete('/api/account', requireUser, authLimiter, async (req, res, next) => {
  try {
    const password = String(req.body.password || '');
    const userRow = await pool.query('SELECT password_hash FROM users WHERE id=$1', [req.user.id]);
    if (!userRow.rows[0] || !(await verifyPassword(password, userRow.rows[0].password_hash))) return res.status(401).json({ error: 'invalid_password' });
    const links = await pool.query('SELECT short_code FROM links WHERE user_id=$1 AND disabled_at IS NULL', [req.user.id]);
    for (const link of links.rows) {
      try { await deleteShortUrl(link.short_code); } catch (error) { if (error?.status !== 404) throw error; }
    }
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
  } catch (error) {
    next(error);
  }
});

async function loadOwnedLink(req, res, next) {
  try {
    const { rows } = await pool.query('SELECT * FROM links WHERE id=$1 AND user_id=$2', [req.params.id, req.user.id]);
    if (!rows[0]) return res.status(404).json({ error: 'link_not_found' });
    req.link = rows[0];
    next();
  } catch (error) {
    next(error);
  }
}

app.get('/api/links', requireUser, async (req, res, next) => {
  try {
    const limit = Math.min(Math.max(Number(req.query.limit) || 50, 1), 100);
    const { rows } = await pool.query(
      `SELECT id,short_code,long_url,title,custom_slug,created_at,updated_at,disabled_at
       FROM links WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`,
      [req.user.id, limit]
    );
    res.json({ links: rows.map(row => ({ ...row, short_url: `${config.publicShortBaseUrl}/${row.short_code}` })) });
  } catch (error) {
    next(error);
  }
});

app.post('/api/links', requireUser, writeLimiter, async (req, res, next) => {
  try {
    const plan = plans[req.user.plan] || plans.free;
    const countResult = await pool.query('SELECT COUNT(*)::int AS count FROM links WHERE user_id=$1 AND disabled_at IS NULL', [req.user.id]);
    if (countResult.rows[0].count >= plan.links) {
      return res.status(402).json({ error: 'plan_limit_reached', limit: plan.links, plan: req.user.plan });
    }

    let longUrl;
    let customSlug;
    try {
      longUrl = normalizeHttpUrl(req.body.longUrl);
      customSlug = normalizeSlug(req.body.customSlug);
    } catch (error) {
      return res.status(400).json({ error: 'invalid_link', message: error.message });
    }
    const title = cleanTitle(req.body.title);
    const reputation = await checkUrlReputation(longUrl);
    if (reputation.threats.length) {
      await audit(req.user.id, 'link.blocked_unsafe', null, { hostname: new URL(longUrl).hostname, threats: reputation.threats });
      return res.status(422).json({ error: 'unsafe_destination', message: 'That destination is flagged as unsafe.', threats: reputation.threats });
    }
    const upstream = await createShortUrl({ longUrl, customSlug, title });
    const shortCode = upstream.shortCode;
    if (!shortCode) throw new Error('Shlink did not return a shortCode');

    const id = crypto.randomUUID();
    try {
      const result = await pool.query(
        `INSERT INTO links(id,user_id,short_code,long_url,title,custom_slug,shlink_domain)
         VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *`,
        [id, req.user.id, shortCode, longUrl, title, customSlug, upstream.domain || null]
      );
      await audit(req.user.id, 'link.created', id, { shortCode });
      res.status(201).json({ link: { ...result.rows[0], short_url: `${config.publicShortBaseUrl}/${shortCode}`, visits: upstream.visitsSummary || { total: 0, nonBots: 0, bots: 0 } } });
    } catch (dbError) {
      try { await deleteShortUrl(shortCode); } catch { /* compensation best-effort */ }
      throw dbError;
    }
  } catch (error) {
    if (error?.status === 400 || error?.status === 409) return res.status(409).json({ error: 'short_url_rejected', message: error.message });
    next(error);
  }
});

app.get('/api/links/:id/stats', requireUser, loadOwnedLink, async (req, res, next) => {
  try {
    const upstream = await getShortUrl(req.link.short_code);
    res.json({
      shortCode: req.link.short_code,
      shortUrl: `${config.publicShortBaseUrl}/${req.link.short_code}`,
      visits: upstream.visitsSummary || { total: 0, nonBots: 0, bots: 0 },
      title: upstream.title || req.link.title,
      longUrl: upstream.longUrl || req.link.long_url
    });
  } catch (error) {
    next(error);
  }
});

app.get('/api/links/:id/visits', requireUser, loadOwnedLink, async (req, res, next) => {
  try {
    const page = Math.max(Number(req.query.page) || 1, 1);
    const data = await getVisits(req.link.short_code, page, 50);
    res.json(data);
  } catch (error) {
    next(error);
  }
});

app.get('/api/links/:id/qr.svg', requireUser, loadOwnedLink, async (req, res, next) => {
  try {
    const svg = await QRCode.toString(`${config.publicShortBaseUrl}/${req.link.short_code}`, { type: 'svg', margin: 1, errorCorrectionLevel: 'M' });
    res.type('image/svg+xml').send(svg);
  } catch (error) {
    next(error);
  }
});

app.patch('/api/links/:id', requireUser, writeLimiter, loadOwnedLink, async (req, res, next) => {
  try {
    let longUrl;
    try { longUrl = normalizeHttpUrl(req.body.longUrl ?? req.link.long_url); }
    catch (error) { return res.status(400).json({ error: 'invalid_link', message: error.message }); }
    const title = req.body.title === undefined ? req.link.title : cleanTitle(req.body.title);
    await editShortUrl(req.link.short_code, { longUrl, title });
    const { rows } = await pool.query('UPDATE links SET long_url=$1,title=$2,updated_at=NOW() WHERE id=$3 RETURNING *', [longUrl, title, req.link.id]);
    await audit(req.user.id, 'link.updated', req.link.id, { shortCode: req.link.short_code });
    res.json({ link: { ...rows[0], short_url: `${config.publicShortBaseUrl}/${req.link.short_code}` } });
  } catch (error) { next(error); }
});

app.delete('/api/links/:id', requireUser, writeLimiter, loadOwnedLink, async (req, res, next) => {
  try {
    if (!req.link.disabled_at) {
      await deleteShortUrl(req.link.short_code);
      await pool.query('UPDATE links SET disabled_at=NOW(),updated_at=NOW() WHERE id=$1', [req.link.id]);
      await audit(req.user.id, 'link.disabled', req.link.id, { shortCode: req.link.short_code });
    }
    res.status(204).end();
  } catch (error) {
    if (error?.status === 404) {
      await pool.query('UPDATE links SET disabled_at=COALESCE(disabled_at,NOW()),updated_at=NOW() WHERE id=$1', [req.link.id]);
      return res.status(204).end();
    }
    next(error);
  }
});

app.post('/api/report', reportLimiter, async (req, res, next) => {
  try {
    const shortCode = String(req.body.shortCode || '').trim().slice(0, 128);
    const reason = String(req.body.reason || '').trim().slice(0, 2000);
    const reporterEmail = normalizeEmail(req.body.email || '') || null;
    if (!shortCode || !reason) return res.status(400).json({ error: 'short_code_and_reason_required' });
    if (reporterEmail && !validEmail(reporterEmail)) return res.status(400).json({ error: 'invalid_email' });
    const linkResult = await pool.query('SELECT id FROM links WHERE short_code=$1 LIMIT 1', [shortCode]);
    const id = crypto.randomUUID();
    await pool.query('INSERT INTO abuse_reports(id,link_id,short_code,reporter_email,reason) VALUES($1,$2,$3,$4,$5)', [id, linkResult.rows[0]?.id || null, shortCode, reporterEmail, reason]);
    await audit(req.user?.id || null, 'abuse.reported', id, { shortCode });
    res.status(201).json({ ok: true, reportId: id });
  } catch (error) { next(error); }
});

app.get('/api/admin/reports', requireAdmin, async (_req, res, next) => {
  try {
    const { rows } = await pool.query(`
      SELECT r.*,l.long_url,l.user_id FROM abuse_reports r
      LEFT JOIN links l ON l.id=r.link_id
      ORDER BY CASE WHEN r.status='open' THEN 0 ELSE 1 END, r.created_at DESC LIMIT 200
    `);
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

app.post('/api/billing/checkout', requireUser, writeLimiter, async (req, res, next) => {
  try { const session = await createCheckout(req.user); res.json({ url: session.url }); } catch (error) { next(error); }
});
app.post('/api/billing/portal', requireUser, writeLimiter, async (req, res, next) => {
  try { const session = await createPortal(req.user); res.json({ url: session.url }); } catch (error) { next(error); }
});

app.use('/assets', express.static(publicDir, { maxAge: config.env === 'production' ? '1h' : 0, index: false }));
app.get('/favicon.svg', (_req, res) => res.sendFile(path.join(publicDir, 'favicon.svg')));
app.get('/robots.txt', (_req, res) => res.sendFile(path.join(publicDir, 'robots.txt')));
app.get('/app', (_req, res) => res.sendFile(path.join(publicDir, 'app.html')));
app.get('/login', (_req, res) => res.redirect(302, '/app#login'));
app.get('/signup', (_req, res) => res.redirect(302, '/app#signup'));
app.get('/report', (_req, res) => res.sendFile(path.join(publicDir, 'report.html')));
app.get('/privacy', (_req, res) => res.sendFile(path.join(publicDir, 'privacy.html')));
app.get('/terms', (_req, res) => res.sendFile(path.join(publicDir, 'terms.html')));
app.get('/', (_req, res) => res.sendFile(path.join(publicDir, 'index.html')));

app.use((req, res) => {
  if (req.path.startsWith('/api/')) return res.status(404).json({ error: 'not_found' });
  res.status(404).sendFile(path.join(publicDir, '404.html'));
});

app.use((error, _req, res, _next) => {
  console.error(error);
  const status = Number(error?.status) >= 400 && Number(error?.status) < 500 ? Number(error.status) : 500;
  const message = status < 500 ? error.message : 'Something went wrong';
  res.status(status).json({ error: status === 500 ? 'internal_error' : 'request_failed', message });
});

await migrate();
await cleanupExpiredSessions();
setInterval(() => cleanupExpiredSessions().catch(console.error), 6 * 60 * 60_000).unref();

app.listen(config.port, '0.0.0.0', () => {
  console.log(`QH8Z listening on :${config.port} (${config.env})`);
});

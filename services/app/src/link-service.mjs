import crypto from 'node:crypto';
import { config, plans } from './config.mjs';
import { pool, audit } from './db.mjs';
import { createShortUrl, editShortUrl, deleteShortUrl, getShortUrl } from './shlink.mjs';
import { checkUrlReputation } from './reputation.mjs';
import { assertDestinationAllowed, assertResolvedDestinationAllowed } from './destination.mjs';
import { normalizeHttpUrl, normalizeSlug, cleanTitle } from './validation.mjs';

const ADVANCED_FEATURE_ERROR = 'Expiry, max-visit controls, bulk creation, and developer API access require QH8Z Pro.';

export function isPro(user) { return user?.plan === 'pro'; }

export function requireProFeature(user) {
  if (isPro(user)) return;
  const error = new Error(ADVANCED_FEATURE_ERROR);
  error.status = 402;
  error.code = 'feature_requires_pro';
  throw error;
}

export function normalizeTags(value) {
  const raw = Array.isArray(value) ? value : String(value || '').split(',');
  const tags = [...new Set(raw.map(tag => String(tag || '').trim().toLowerCase()).filter(Boolean))];
  if (tags.length > 12) throw Object.assign(new Error('Use at most 12 tags per link.'), { status: 400, code: 'too_many_tags' });
  for (const tag of tags) {
    if (!/^[a-z0-9][a-z0-9_-]{0,31}$/.test(tag)) throw Object.assign(new Error(`Invalid tag: ${tag}`), { status: 400, code: 'invalid_tag' });
  }
  return tags;
}

export function normalizePagination(query = {}) {
  const rawLimit = Number(query.limit);
  const rawOffset = Number(query.offset);
  const limit = Number.isSafeInteger(rawLimit) && rawLimit >= 1 ? Math.min(rawLimit, 100) : 25;
  const offset = Number.isSafeInteger(rawOffset) && rawOffset >= 0 ? Math.min(rawOffset, 1_000_000) : 0;
  return { limit, offset };
}

function normalizeNotes(value) {
  const notes = String(value || '').trim();
  if (notes.length > 2000) throw Object.assign(new Error('Notes must be 2,000 characters or fewer.'), { status: 400, code: 'notes_too_long' });
  return notes || null;
}

function normalizeExpiresAt(value) {
  if (value == null || String(value).trim() === '') return null;
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) throw Object.assign(new Error('Expiration must be a valid date.'), { status: 400, code: 'invalid_expiration' });
  if (date.getTime() <= Date.now() + 60_000) throw Object.assign(new Error('Expiration must be at least one minute in the future.'), { status: 400, code: 'invalid_expiration' });
  if (date.getTime() > Date.now() + 5 * 365 * 24 * 60 * 60_000) throw Object.assign(new Error('Expiration cannot be more than five years away.'), { status: 400, code: 'invalid_expiration' });
  return date.toISOString();
}

function normalizeMaxVisits(value) {
  if (value == null || String(value).trim() === '') return null;
  const n = Number(value);
  if (!Number.isSafeInteger(n) || n < 1 || n > 10_000_000) throw Object.assign(new Error('Max visits must be an integer from 1 to 10,000,000.'), { status: 400, code: 'invalid_max_visits' });
  return n;
}

export function publicLink(row) {
  const expired = Boolean(row.expires_at && new Date(row.expires_at).getTime() <= Date.now());
  const state = row.disabled_at ? 'disabled' : row.archived_at ? 'archived' : expired ? 'expired' : 'active';
  return { ...row, tags: Array.isArray(row.tags) ? row.tags : [], short_url: `${config.publicShortBaseUrl}/${row.short_code}`, state };
}

export function editConsumesPlanSlot(existing, nextExpiresAt, now = Date.now()) {
  if (!existing?.expires_at) return false;
  const previousExpiry = new Date(existing.expires_at).getTime();
  if (!Number.isFinite(previousExpiry) || previousExpiry > now) return false;
  if (!nextExpiresAt) return true;
  const nextExpiry = new Date(nextExpiresAt).getTime();
  return Number.isFinite(nextExpiry) && nextExpiry > now;
}

export async function validateDestination(longUrl, userId, targetId = null) {
  assertDestinationAllowed(longUrl);
  if (config.publicLaunchMode) await assertResolvedDestinationAllowed(longUrl);
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

export function normalizeLinkFields(input = {}, existing = null, user = null) {
  let longUrl;
  try { longUrl = normalizeHttpUrl(input.longUrl ?? existing?.long_url); }
  catch (error) { throw Object.assign(new Error(error.message), { status: 400, code: 'invalid_link' }); }
  const customSlug = existing ? existing.custom_slug : normalizeSlug(input.customSlug);
  const title = input.title === undefined && existing ? existing.title : cleanTitle(input.title);
  const notes = input.notes === undefined && existing ? existing.notes : normalizeNotes(input.notes);
  const tags = input.tags === undefined && existing ? (existing.tags || []) : normalizeTags(input.tags);
  const expiresAt = input.expiresAt === undefined && existing ? existing.expires_at : normalizeExpiresAt(input.expiresAt);
  const maxVisits = input.maxVisits === undefined && existing ? existing.max_visits : normalizeMaxVisits(input.maxVisits);
  const changingExpiry = input.expiresAt !== undefined && Boolean(expiresAt);
  const changingMaxVisits = input.maxVisits !== undefined && Boolean(maxVisits);
  if ((changingExpiry || changingMaxVisits) && user && !isPro(user)) requireProFeature(user);
  return { longUrl, customSlug, title, notes, tags, expiresAt, maxVisits };
}

async function friendlyPlanLimit(user) {
  const plan = plans[user.plan] || plans.free;
  const { rows } = await pool.query(
    `SELECT COUNT(*)::int AS count FROM links
     WHERE user_id=$1 AND disabled_at IS NULL AND (expires_at IS NULL OR expires_at>NOW())`,
    [user.id]
  );
  if (rows[0].count >= plan.links) {
    const error = new Error(`Your ${plan.name} plan allows ${plan.links.toLocaleString()} active links.`);
    error.status = 402;
    error.code = 'plan_limit_reached';
    error.limit = plan.links;
    throw error;
  }
}

export async function createLink(user, input = {}) {
  await friendlyPlanLimit(user);
  const fields = normalizeLinkFields(input, null, user);
  await validateDestination(fields.longUrl, user.id);
  const upstream = await createShortUrl({ longUrl: fields.longUrl, customSlug: fields.customSlug, title: fields.title, tags: fields.tags, validUntil: fields.expiresAt, maxVisits: fields.maxVisits });
  const shortCode = upstream.shortCode;
  if (!shortCode) throw new Error('Shlink did not return a shortCode');
  const id = crypto.randomUUID();
  try {
    const { rows } = await pool.query(
      `INSERT INTO links(id,user_id,short_code,long_url,title,custom_slug,shlink_domain,notes,tags,expires_at,max_visits)
       VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11) RETURNING *`,
      [id, user.id, shortCode, fields.longUrl, fields.title, fields.customSlug, upstream.domain || null, fields.notes, JSON.stringify(fields.tags), fields.expiresAt, fields.maxVisits]
    );
    await audit(user.id, 'link.created', id, { shortCode, tags: fields.tags, expiresAt: fields.expiresAt, maxVisits: fields.maxVisits });
    return { ...publicLink(rows[0]), visits: upstream.visitsSummary || { total: 0, nonBots: 0, bots: 0 } };
  } catch (error) {
    try { await deleteShortUrl(shortCode); } catch {}
    throw error;
  }
}

export async function getOwnedLink(userId, id) {
  const { rows } = await pool.query('SELECT * FROM links WHERE id=$1 AND user_id=$2', [id, userId]);
  return rows[0] || null;
}

export async function listLinks(userId, query = {}) {
  const { limit, offset } = normalizePagination(query);
  const q = String(query.q || '').trim().slice(0, 200);
  const status = ['all','active','disabled','archived','expired'].includes(String(query.status || 'all')) ? String(query.status || 'all') : 'all';
  const tag = String(query.tag || '').trim().toLowerCase().slice(0, 32);
  const params = [userId];
  const where = ['user_id=$1'];
  if (q) { params.push(`%${q}%`); where.push(`(short_code ILIKE $${params.length} OR long_url ILIKE $${params.length} OR COALESCE(title,'') ILIKE $${params.length} OR COALESCE(notes,'') ILIKE $${params.length})`); }
  if (tag) { params.push(JSON.stringify([tag])); where.push(`tags @> $${params.length}::jsonb`); }
  if (status === 'active') where.push('disabled_at IS NULL AND archived_at IS NULL AND (expires_at IS NULL OR expires_at>NOW())');
  if (status === 'disabled') where.push('disabled_at IS NOT NULL');
  if (status === 'archived') where.push('archived_at IS NOT NULL AND disabled_at IS NULL');
  if (status === 'expired') where.push('disabled_at IS NULL AND expires_at IS NOT NULL AND expires_at<=NOW()');
  const count = await pool.query(`SELECT COUNT(*)::int AS total FROM links WHERE ${where.join(' AND ')}`, params);
  params.push(limit, offset);
  const { rows } = await pool.query(
    `SELECT id,short_code,long_url,title,custom_slug,notes,tags,expires_at,max_visits,created_at,updated_at,disabled_at,archived_at,reputation_status
     FROM links WHERE ${where.join(' AND ')} ORDER BY created_at DESC LIMIT $${params.length - 1} OFFSET $${params.length}`,
    params
  );
  return { links: rows.map(publicLink), total: count.rows[0].total, limit, offset, hasMore: offset + rows.length < count.rows[0].total };
}

export async function updateLink(user, link, input = {}) {
  if (link.disabled_at) throw Object.assign(new Error('Restore the link before editing it.'), { status: 409, code: 'link_disabled' });
  const fields = normalizeLinkFields(input, link, user);
  if (editConsumesPlanSlot(link, fields.expiresAt)) await friendlyPlanLimit(user);
  await validateDestination(fields.longUrl, user.id, link.id);
  await editShortUrl(link.short_code, { longUrl: fields.longUrl, title: fields.title, tags: fields.tags, validUntil: fields.expiresAt, maxVisits: fields.maxVisits });
  const { rows } = await pool.query(
    `UPDATE links SET long_url=$1,title=$2,notes=$3,tags=$4::jsonb,expires_at=$5,max_visits=$6,updated_at=NOW()
     WHERE id=$7 RETURNING *`,
    [fields.longUrl, fields.title, fields.notes, JSON.stringify(fields.tags), fields.expiresAt, fields.maxVisits, link.id]
  );
  await audit(user.id, 'link.updated', link.id, { shortCode: link.short_code });
  return publicLink(rows[0]);
}

export async function disableLink(user, link, eventType = 'link.disabled') {
  if (!link.disabled_at) {
    try { await deleteShortUrl(link.short_code); } catch (error) { if (error?.status !== 404) throw error; }
    await pool.query('UPDATE links SET disabled_at=NOW(),updated_at=NOW() WHERE id=$1', [link.id]);
    await audit(user.id, eventType, link.id, { shortCode: link.short_code });
  }
}

export async function restoreLink(user, link) {
  if (!link.disabled_at) return publicLink(link);
  await friendlyPlanLimit(user);
  const storedExpiry = link.expires_at ? new Date(link.expires_at) : null;
  const expiryStillUsable = storedExpiry && Number.isFinite(storedExpiry.getTime()) && storedExpiry.getTime() > Date.now() + 60_000;
  const restoreExpiry = isPro(user) && expiryStillUsable ? storedExpiry.toISOString() : null;
  const restoreMaxVisits = isPro(user) ? link.max_visits : null;
  const fields = { longUrl: normalizeHttpUrl(link.long_url), title: link.title, notes: link.notes, tags: Array.isArray(link.tags) ? link.tags : [], expiresAt: restoreExpiry, maxVisits: restoreMaxVisits };
  await validateDestination(fields.longUrl, user.id, link.id);
  await createShortUrl({ longUrl: fields.longUrl, customSlug: link.short_code, title: fields.title, tags: fields.tags, validUntil: fields.expiresAt, maxVisits: fields.maxVisits, allowOwnedLinkId: link.id });
  try {
    const { rows } = await pool.query('UPDATE links SET disabled_at=NULL,expires_at=$1,max_visits=$2,updated_at=NOW() WHERE id=$3 RETURNING *', [fields.expiresAt, fields.maxVisits, link.id]);
    await audit(user.id, 'link.restored', link.id, { shortCode: link.short_code, advancedControlsRetained: Boolean(fields.expiresAt || fields.maxVisits) });
    return publicLink(rows[0]);
  } catch (error) {
    try { await deleteShortUrl(link.short_code); } catch {}
    throw error;
  }
}

export async function setArchived(user, link, archived) {
  const { rows } = await pool.query(`UPDATE links SET archived_at=CASE WHEN $1 THEN COALESCE(archived_at,NOW()) ELSE NULL END,updated_at=NOW() WHERE id=$2 RETURNING *`, [Boolean(archived), link.id]);
  await audit(user.id, archived ? 'link.archived' : 'link.unarchived', link.id, { shortCode: link.short_code });
  return publicLink(rows[0]);
}

export async function bulkCreateLinks(user, items) {
  requireProFeature(user);
  if (!Array.isArray(items) || !items.length) throw Object.assign(new Error('Provide at least one link.'), { status: 400, code: 'links_required' });
  if (items.length > 100) throw Object.assign(new Error('Bulk creation is limited to 100 links per request.'), { status: 400, code: 'bulk_limit' });
  const results = new Array(items.length);
  for (let offset = 0; offset < items.length; offset += 5) {
    await Promise.all(items.slice(offset, offset + 5).map(async (item, index) => {
      const absoluteIndex = offset + index;
      try { results[absoluteIndex] = { ok: true, link: await createLink(user, item) }; }
      catch (error) { results[absoluteIndex] = { ok: false, error: error.code || 'create_failed', message: error.message }; }
    }));
  }
  return { results, created: results.filter(x => x.ok).length, failed: results.filter(x => !x.ok).length };
}

function spreadsheetSafe(value) {
  let text = String(value ?? '');
  if (/^[=+\-@\t\r]/.test(text)) text = `'${text}`;
  return text;
}

export function linksToCsv(rows) {
  const columns = ['short_url','long_url','title','tags','notes','state','created_at','expires_at','max_visits'];
  const quote = value => `"${spreadsheetSafe(value).replaceAll('"', '""')}"`;
  const lines = [columns.join(',')];
  for (const raw of rows) {
    const row = publicLink(raw);
    lines.push([row.short_url,row.long_url,row.title,(row.tags || []).join('|'),row.notes,row.state,row.created_at,row.expires_at,row.max_visits].map(quote).join(','));
  }
  return `${lines.join('\n')}\n`;
}

export async function getLinkStats(link) {
  if (link.disabled_at) {
    return {
      shortCode: link.short_code,
      shortUrl: `${config.publicShortBaseUrl}/${link.short_code}`,
      visits: { total: 0, nonBots: 0, bots: 0 },
      title: link.title,
      longUrl: link.long_url,
      maxVisits: link.max_visits,
      validUntil: link.expires_at,
      tags: link.tags || [],
      unavailable: true,
    };
  }
  const upstream = await getShortUrl(link.short_code);
  return {
    shortCode: link.short_code,
    shortUrl: `${config.publicShortBaseUrl}/${link.short_code}`,
    visits: upstream.visitsSummary || { total: 0, nonBots: 0, bots: 0 },
    title: upstream.title || link.title,
    longUrl: upstream.longUrl || link.long_url,
    maxVisits: upstream.maxVisits ?? link.max_visits,
    validUntil: upstream.validUntil ?? link.expires_at,
    tags: upstream.tags || link.tags || [],
  };
}
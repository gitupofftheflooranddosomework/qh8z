import test from 'node:test';
import assert from 'node:assert/strict';

process.env.PUBLIC_SHORT_BASE_URL = 'https://qh8z.test';
process.env.REPUTATION_WORKER_MINUTES = '0';

const { normalizeTags, normalizeLinkFields, publicLink } = await import('../src/link-service.mjs');

test('normalizes tags', () => {
  assert.deepEqual(normalizeTags('summer,social,summer,utm_1'), ['summer', 'social', 'utm_1']);
});

test('Free allows notes and tags', () => {
  const fields = normalizeLinkFields({ longUrl: 'https://example.com/x', tags: 'one,two', notes: 'private note' }, null, { plan: 'free' });
  assert.deepEqual(fields.tags, ['one', 'two']);
  assert.equal(fields.notes, 'private note');
});

test('Free cannot add expiry or max visits', () => {
  assert.throws(() => normalizeLinkFields({ longUrl: 'https://example.com/x', expiresAt: new Date(Date.now() + 3600000).toISOString() }, null, { plan: 'free' }), error => error?.code === 'feature_requires_pro');
  assert.throws(() => normalizeLinkFields({ longUrl: 'https://example.com/x', maxVisits: 100 }, null, { plan: 'free' }), error => error?.code === 'feature_requires_pro');
});

test('downgraded users can edit basic fields without changing existing advanced values', () => {
  const existing = { long_url: 'https://example.com/old', title: 'Old', notes: 'Old', tags: ['paid'], custom_slug: 'launch', expires_at: new Date(Date.now() + 86400000).toISOString(), max_visits: 500 };
  const fields = normalizeLinkFields({ title: 'New', notes: 'New notes', tags: 'paid,keep' }, existing, { plan: 'free' });
  assert.equal(fields.title, 'New');
  assert.equal(fields.maxVisits, 500);
  assert.deepEqual(fields.tags, ['paid', 'keep']);
});

test('Pro accepts advanced controls', () => {
  const expiresAt = new Date(Date.now() + 86400000).toISOString();
  const fields = normalizeLinkFields({ longUrl: 'https://example.com', expiresAt, maxVisits: 2500 }, null, { plan: 'pro' });
  assert.equal(fields.maxVisits, 2500);
  assert.equal(new Date(fields.expiresAt).getTime(), new Date(expiresAt).getTime());
});

test('public state distinguishes active archived expired and disabled', () => {
  const base = { short_code: 'abc1234', tags: [] };
  assert.equal(publicLink(base).state, 'active');
  assert.equal(publicLink({ ...base, archived_at: new Date().toISOString() }).state, 'archived');
  assert.equal(publicLink({ ...base, expires_at: new Date(Date.now() - 1000).toISOString() }).state, 'expired');
  assert.equal(publicLink({ ...base, disabled_at: new Date().toISOString() }).state, 'disabled');
});

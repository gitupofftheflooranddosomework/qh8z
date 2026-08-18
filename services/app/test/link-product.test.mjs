import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeTags, normalizeLinkFields, normalizePagination, publicLink, linksToCsv } from '../src/link-service.mjs';

test('tags are normalized, deduplicated, and validated', () => {
  assert.deepEqual(normalizeTags('Launch, launch, Summer_26'), ['launch', 'summer_26']);
  assert.throws(() => normalizeTags('not valid!'), /Invalid tag/);
  assert.throws(() => normalizeTags(Array.from({ length: 13 }, (_, i) => `tag${i}`)), /at most 12/);
});

test('Free cannot add Pro redirect controls', () => {
  assert.throws(
    () => normalizeLinkFields({ longUrl: 'https://example.com', maxVisits: 10 }, null, { plan: 'free' }),
    error => error?.code === 'feature_requires_pro' && error?.status === 402,
  );
});

test('downgraded Free account can preserve existing Pro controls during ordinary edits', () => {
  const existing = {
    long_url: 'https://example.com/original', custom_slug: 'saved', title: 'Old', notes: 'keep', tags: ['campaign'],
    expires_at: '2027-01-01T00:00:00.000Z', max_visits: 500,
  };
  const fields = normalizeLinkFields({ title: 'New title' }, existing, { plan: 'free' });
  assert.equal(fields.title, 'New title');
  assert.equal(fields.expiresAt, existing.expires_at);
  assert.equal(fields.maxVisits, 500);
});

test('pagination rejects non-integers and unreasonable offsets without leaking DB errors', () => {
  assert.deepEqual(normalizePagination({}), { limit: 25, offset: 0 });
  assert.deepEqual(normalizePagination({ limit: '100', offset: '50' }), { limit: 100, offset: 50 });
  assert.deepEqual(normalizePagination({ limit: 'Infinity', offset: '1.5' }), { limit: 25, offset: 0 });
  assert.deepEqual(normalizePagination({ limit: '-5', offset: '-1' }), { limit: 25, offset: 0 });
  assert.deepEqual(normalizePagination({ limit: '1000', offset: '99999999' }), { limit: 100, offset: 1000000 });
});

test('link state precedence is disabled, archived, expired, active', () => {
  const base = { short_code: 'abc', long_url: 'https://example.com', tags: [] };
  assert.equal(publicLink({ ...base, disabled_at: new Date(), archived_at: new Date(), expires_at: '2000-01-01' }).state, 'disabled');
  assert.equal(publicLink({ ...base, disabled_at: null, archived_at: new Date(), expires_at: '2000-01-01' }).state, 'archived');
  assert.equal(publicLink({ ...base, disabled_at: null, archived_at: null, expires_at: '2000-01-01' }).state, 'expired');
  assert.equal(publicLink({ ...base, disabled_at: null, archived_at: null, expires_at: null }).state, 'active');
});

test('CSV export quotes commas and quotes without corrupting rows', () => {
  const csv = linksToCsv([{ short_code: 'csv', long_url: 'https://example.com/a,b', title: 'Say "hello"', tags: ['a','b'], notes: 'x,y', created_at: '2026-08-18T00:00:00Z' }]);
  assert.match(csv, /"https:\/\/example\.com\/a,b"/);
  assert.match(csv, /"Say ""hello"""/);
  assert.match(csv, /"x,y"/);
});

test('CSV export neutralizes spreadsheet formula prefixes', () => {
  const csv = linksToCsv([{ short_code: 'formula', long_url: 'https://example.com', title: '=HYPERLINK("https://evil.example")', tags: ['safe'], notes: '+SUM(1,1)', created_at: '2026-08-18T00:00:00Z' }]);
  assert.match(csv, /"'=HYPERLINK\(""https:\/\/evil\.example""\)"/);
  assert.match(csv, /"'\+SUM\(1,1\)"/);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { trackedLinkMatches } from '../src/consistency.mjs';

test('tracked link comparison normalizes equivalent absolute URLs', () => {
  assert.equal(trackedLinkMatches({ long_url: 'https://example.com/' }, { longUrl: 'https://example.com' }), true);
  assert.equal(trackedLinkMatches({ long_url: 'https://example.com/a?b=1' }, { longUrl: 'https://example.com/a?b=1' }), true);
});

test('tracked link comparison detects destination divergence and malformed upstream values', () => {
  assert.equal(trackedLinkMatches({ long_url: 'https://example.com/one' }, { longUrl: 'https://example.com/two' }), false);
  assert.equal(trackedLinkMatches({ long_url: 'https://example.com/one' }, { longUrl: 'not a url' }), false);
  assert.equal(trackedLinkMatches({ long_url: 'https://example.com/one' }, null), false);
});

test('tracked link comparison covers the full Shlink redirect contract', () => {
  const link = {
    long_url: 'https://example.com/campaign',
    title: 'Campaign',
    tags: ['launch', 'paid'],
    expires_at: '2027-01-01T00:00:00.000Z',
    max_visits: 500,
  };
  const upstream = {
    longUrl: 'https://example.com/campaign',
    title: 'Campaign',
    tags: ['paid', 'launch'],
    meta: { validUntil: '2027-01-01T00:00:00Z', maxVisits: 500 },
  };

  assert.equal(trackedLinkMatches(link, upstream), true);
  assert.equal(trackedLinkMatches(link, { ...upstream, title: 'Changed' }), false);
  assert.equal(trackedLinkMatches(link, { ...upstream, tags: ['launch'] }), false);
  assert.equal(trackedLinkMatches(link, { ...upstream, meta: { ...upstream.meta, validUntil: null } }), false);
  assert.equal(trackedLinkMatches(link, { ...upstream, meta: { ...upstream.meta, maxVisits: null } }), false);
});

test('tracked link comparison tolerates legacy/root control fields', () => {
  const link = { long_url: 'https://example.com', expires_at: null, max_visits: 10 };
  assert.equal(trackedLinkMatches(link, { longUrl: 'https://example.com', maxVisits: 10 }), true);
});

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

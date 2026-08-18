import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeApiScopes, normalizeApiTokenExpiryDays } from '../src/api-tokens.mjs';

test('API token scopes default to read/write and deduplicate', () => {
  assert.deepEqual(normalizeApiScopes(), ['links:read', 'links:write']);
  assert.deepEqual(normalizeApiScopes(['links:read', 'links:read']), ['links:read']);
});

test('empty scope set gets least-privilege read access', () => {
  assert.deepEqual(normalizeApiScopes([]), ['links:read']);
});

test('unknown or malformed API scopes are rejected', () => {
  assert.throws(() => normalizeApiScopes('links:write'), error => error?.code === 'invalid_api_scopes');
  assert.throws(() => normalizeApiScopes(['links:admin']), error => error?.code === 'invalid_api_scope');
});

test('API token expiry defaults only when omitted and otherwise validates strictly', () => {
  assert.equal(normalizeApiTokenExpiryDays(), 365);
  assert.equal(normalizeApiTokenExpiryDays(''), 365);
  assert.equal(normalizeApiTokenExpiryDays(1), 1);
  assert.equal(normalizeApiTokenExpiryDays('3650'), 3650);
  for (const value of [0, -1, 3651, 1.5, 'abc']) {
    assert.throws(
      () => normalizeApiTokenExpiryDays(value),
      error => error?.status === 400 && error?.code === 'invalid_api_token_expiry'
    );
  }
});

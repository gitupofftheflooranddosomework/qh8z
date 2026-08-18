import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeApiScopes } from '../src/api-tokens.mjs';

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

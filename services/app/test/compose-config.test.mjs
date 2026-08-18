import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const testDir = path.dirname(fileURLToPath(import.meta.url));
const composePath = path.resolve(testDir, '../../../docker-compose.yml');
const compose = fs.readFileSync(composePath, 'utf8');

test('Shlink runtime does not fetch destination titles behind QH8Z', () => {
  assert.match(compose, /AUTO_RESOLVE_TITLES:\s*['"]false['"]/);
});

test('Shlink keeps visitor addresses anonymized and direct dev port loopback-only', () => {
  assert.match(compose, /ANONYMIZE_REMOTE_ADDR:\s*['"]true['"]/);
  assert.match(compose, /127\.0\.0\.1:\$\{SHLINK_DEV_PORT:-8080\}:8080/);
});

test('Shlink deletion remains unlimited so moderation can remove high-traffic links', () => {
  assert.doesNotMatch(compose, /DELETE_SHORT_URL_THRESHOLD\s*:/);
});

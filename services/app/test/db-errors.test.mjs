import test from 'node:test';
import assert from 'node:assert/strict';
import { translateDbError } from '../src/db.mjs';

test('named PostgreSQL link quota violations become controlled HTTP 402 errors', () => {
  const error = Object.assign(new Error('link plan limit reached'), {
    code: 'P0001',
    constraint: 'qh8z_link_plan_limit',
  });
  const translated = translateDbError(error);
  assert.equal(translated, error);
  assert.equal(translated.status, 402);
  assert.equal(translated.code, 'plan_limit_reached');
  assert.equal(translated.message, 'Your active-link limit has been reached.');
});

test('unrelated PostgreSQL errors are not reclassified', () => {
  const error = Object.assign(new Error('unique violation'), { code: '23505', constraint: 'users_email_key' });
  const translated = translateDbError(error);
  assert.equal(translated.status, undefined);
  assert.equal(translated.code, '23505');
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { hasSessionCookie } from '../src/auth.mjs';

test('malformed percent encoding in the session cookie fails closed', () => {
  assert.equal(hasSessionCookie({ headers: { cookie: 'qh8z_session=%' } }), false);
});

test('malformed unrelated cookies do not hide a valid session cookie', () => {
  assert.equal(hasSessionCookie({ headers: { cookie: 'broken=%; qh8z_session=valid-token' } }), true);
});

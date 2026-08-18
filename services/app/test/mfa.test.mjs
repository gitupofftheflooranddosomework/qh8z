import test from 'node:test';
import assert from 'node:assert/strict';
import { verifyTotp } from '../src/mfa.mjs';

// RFC 6238 SHA-1 test secret "12345678901234567890". The RFC 8-digit value
// at T=59s is 94287082, so the corresponding 6-digit HOTP/TOTP value is 287082.
test('TOTP verifier matches RFC 6238 SHA-1 vector', () => {
  const base32Secret = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ';
  assert.equal(verifyTotp(base32Secret, '287082', 59_000), true);
  assert.equal(verifyTotp(base32Secret, '287083', 59_000), false);
});

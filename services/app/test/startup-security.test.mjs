import test from 'node:test';
import assert from 'node:assert/strict';
import { adminMfaEncryptionProblems } from '../src/startup-security.mjs';

test('admins without MFA enabled do not require an encrypted TOTP secret yet', () => {
  assert.deepEqual(adminMfaEncryptionProblems([{ id: 'admin-1', mfa_enabled_at: null, mfa_secret_enc: null }], () => { throw new Error('should not decrypt'); }), []);
});

test('enabled administrator MFA requires an encrypted secret', () => {
  const problems = adminMfaEncryptionProblems([{ id: 'admin-1', mfa_enabled_at: new Date().toISOString(), mfa_secret_enc: null }], () => 'A'.repeat(32));
  assert.ok(problems.some(problem => problem.includes('without an encrypted secret')));
});

test('startup blocks when the configured key cannot decrypt an enabled admin secret', () => {
  const problems = adminMfaEncryptionProblems([{ id: 'admin-1', mfa_enabled_at: new Date().toISOString(), mfa_secret_enc: 'ciphertext' }], () => { throw new Error('bad key'); });
  assert.ok(problems.some(problem => problem.includes('cannot be decrypted')));
});

test('startup accepts a decryptable base32 administrator secret', () => {
  const problems = adminMfaEncryptionProblems([{ id: 'admin-1', mfa_enabled_at: new Date().toISOString(), mfa_secret_enc: 'ciphertext' }], () => 'JBSWY3DPEHPK3PXPJBSWY3DP');
  assert.deepEqual(problems, []);
});

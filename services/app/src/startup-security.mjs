import { pool } from './db.mjs';
import { decryptMfaSecret } from './mfa.mjs';

export function adminMfaEncryptionProblems(admins) {
  const problems = [];
  for (const admin of admins || []) {
    if (!admin?.mfa_enabled_at) continue;
    if (!admin.mfa_secret_enc) {
      problems.push(`administrator ${admin.id || 'unknown'} has MFA enabled without an encrypted secret`);
      continue;
    }
    try {
      const secret = decryptMfaSecret(admin.mfa_secret_enc);
      if (!/^[A-Z2-7]{16,}$/i.test(String(secret || ''))) problems.push(`administrator ${admin.id || 'unknown'} has an invalid decrypted MFA secret`);
    } catch {
      problems.push(`administrator ${admin.id || 'unknown'} MFA secret cannot be decrypted with MFA_ENCRYPTION_KEY`);
    }
  }
  return problems;
}

export async function validateAdminMfaEncryption() {
  const { rows } = await pool.query('SELECT id,mfa_enabled_at,mfa_secret_enc FROM users WHERE is_admin=TRUE');
  return adminMfaEncryptionProblems(rows);
}

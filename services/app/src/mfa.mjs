import crypto from 'node:crypto';
import { config } from './config.mjs';
import { pool } from './db.mjs';

const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

function encryptionKey() {
  const raw = String(config.mfaEncryptionKey || '');
  if (!/^[0-9a-fA-F]{64}$/.test(raw)) throw new Error('MFA_ENCRYPTION_KEY must be 32 bytes encoded as 64 hex characters');
  return Buffer.from(raw, 'hex');
}

function base32Encode(buffer) {
  let bits = 0;
  let value = 0;
  let out = '';
  for (const byte of buffer) {
    value = (value << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      out += ALPHABET[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += ALPHABET[(value << (5 - bits)) & 31];
  return out;
}

function base32Decode(input) {
  const clean = String(input || '').toUpperCase().replace(/[^A-Z2-7]/g, '');
  let bits = 0;
  let value = 0;
  const bytes = [];
  for (const char of clean) {
    const idx = ALPHABET.indexOf(char);
    if (idx < 0) continue;
    value = (value << 5) | idx;
    bits += 5;
    if (bits >= 8) {
      bytes.push((value >>> (bits - 8)) & 255);
      bits -= 8;
    }
  }
  return Buffer.from(bytes);
}

function encrypt(secret) {
  const iv = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv('aes-256-gcm', encryptionKey(), iv);
  const encrypted = Buffer.concat([cipher.update(secret, 'utf8'), cipher.final()]);
  const tag = cipher.getAuthTag();
  return [iv, tag, encrypted].map(x => x.toString('base64url')).join('.');
}

function decrypt(payload) {
  const [ivRaw, tagRaw, dataRaw] = String(payload || '').split('.');
  if (!ivRaw || !tagRaw || !dataRaw) throw new Error('Invalid encrypted MFA secret');
  const decipher = crypto.createDecipheriv('aes-256-gcm', encryptionKey(), Buffer.from(ivRaw, 'base64url'));
  decipher.setAuthTag(Buffer.from(tagRaw, 'base64url'));
  return Buffer.concat([decipher.update(Buffer.from(dataRaw, 'base64url')), decipher.final()]).toString('utf8');
}

function codeAt(secret, counter) {
  const key = base32Decode(secret);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const digest = crypto.createHmac('sha1', key).update(buf).digest();
  const offset = digest[digest.length - 1] & 0x0f;
  const n = ((digest[offset] & 0x7f) << 24) | (digest[offset + 1] << 16) | (digest[offset + 2] << 8) | digest[offset + 3];
  return String(n % 1_000_000).padStart(6, '0');
}

export function verifyTotp(secret, code, now = Date.now()) {
  const normalized = String(code || '').replace(/\s+/g, '');
  if (!/^\d{6}$/.test(normalized)) return false;
  const counter = Math.floor(now / 30_000);
  for (const delta of [-1, 0, 1]) {
    const expected = codeAt(secret, counter + delta);
    if (crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(normalized))) return true;
  }
  return false;
}

function recoveryHash(code) {
  return crypto.createHash('sha256').update(String(code || '').replace(/[^A-Za-z0-9]/g, '').toUpperCase()).digest('hex');
}

export function generateMfaSetup(email) {
  const secret = base32Encode(crypto.randomBytes(20));
  const label = encodeURIComponent(`QH8Z:${email}`);
  const issuer = encodeURIComponent('QH8Z');
  return {
    secret,
    encryptedSecret: encrypt(secret),
    otpauthUri: `otpauth://totp/${label}?secret=${secret}&issuer=${issuer}&algorithm=SHA1&digits=6&period=30`,
  };
}

export function generateRecoveryCodes(count = 10) {
  const codes = [];
  for (let i = 0; i < count; i += 1) {
    const raw = crypto.randomBytes(5).toString('hex').toUpperCase();
    codes.push(`${raw.slice(0, 5)}-${raw.slice(5)}`);
  }
  return { codes, hashes: codes.map(recoveryHash) };
}

export async function verifyMfaUser(user, code, consumeRecovery = true) {
  if (!user?.mfa_enabled_at || !user?.mfa_secret_enc) return false;
  const normalized = String(code || '').trim();
  if (verifyTotp(decrypt(user.mfa_secret_enc), normalized)) return true;
  const hash = recoveryHash(normalized);
  const hashes = Array.isArray(user.mfa_recovery_hashes) ? user.mfa_recovery_hashes : [];
  const idx = hashes.indexOf(hash);
  if (idx < 0) return false;
  if (consumeRecovery) {
    const next = hashes.filter((_, i) => i !== idx);
    await pool.query('UPDATE users SET mfa_recovery_hashes=$1::jsonb WHERE id=$2', [JSON.stringify(next), user.id]);
  }
  return true;
}

export function decryptMfaSecret(encrypted) {
  return decrypt(encrypted);
}

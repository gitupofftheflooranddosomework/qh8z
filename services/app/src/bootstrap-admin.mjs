import crypto from 'node:crypto';
import { config } from './config.mjs';
import { pool, migrate, audit } from './db.mjs';
import { hashPassword } from './auth.mjs';
import { validEmail, validPassword } from './validation.mjs';

const password = process.env.BOOTSTRAP_ADMIN_PASSWORD || '';
if (!validEmail(config.adminEmail)) throw new Error('ADMIN_EMAIL must be configured with a real email address');
if (!config.adminBootstrapSecret || config.adminBootstrapSecret.length < 24) throw new Error('ADMIN_BOOTSTRAP_SECRET must be configured');
if (process.env.BOOTSTRAP_ADMIN_SECRET !== config.adminBootstrapSecret) throw new Error('Bootstrap confirmation secret does not match');
if (!validPassword(password)) throw new Error('Admin password must be 10-72 UTF-8 bytes');

await migrate();
const existingAdmin = await pool.query('SELECT id,email FROM users WHERE is_admin=TRUE LIMIT 1');
if (existingAdmin.rows[0]) throw new Error(`An administrator already exists (${existingAdmin.rows[0].email})`);
const existingEmail = await pool.query('SELECT id FROM users WHERE email=$1', [config.adminEmail]);
if (existingEmail.rows[0]) throw new Error('ADMIN_EMAIL already belongs to a non-admin account; resolve it before bootstrap');

const id = crypto.randomUUID();
const passwordHash = await hashPassword(password);
await pool.query(
  `INSERT INTO users(id,email,password_hash,name,is_admin,email_verified_at,terms_accepted_at,terms_version)
   VALUES($1,$2,$3,$4,TRUE,NOW(),NOW(),$5)`,
  [id, config.adminEmail, passwordHash, 'QH8Z Administrator', config.termsVersion]
);
await audit(id, 'admin.bootstrapped', id, { method: 'local_cli' });
console.log(`QH8Z administrator bootstrapped for ${config.adminEmail}. Sign in and enable MFA before public readiness can pass.`);
await pool.end();

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const testDir = path.dirname(fileURLToPath(import.meta.url));
const dbSource = fs.readFileSync(path.resolve(testDir, '../src/db.mjs'), 'utf8');

test('plan-limit trigger only charges operations that consume an active slot', () => {
  assert.match(dbSource, /new_active := NEW\.disabled_at IS NULL/);
  assert.match(dbSource, /old_active := OLD\.disabled_at IS NULL/);
  assert.match(dbSource, /IF old_active THEN\s+RETURN NEW;/);
  assert.match(dbSource, /pg_advisory_xact_lock\(hashtext\(NEW\.user_id\), hashtext\('qh8z-link-plan-limit'\)\)/);
});

test('Stripe subscription entitlement state is durable and user-owned', () => {
  assert.match(dbSource, /CREATE TABLE IF NOT EXISTS stripe_subscriptions/);
  assert.match(dbSource, /subscription_id TEXT PRIMARY KEY/);
  assert.match(dbSource, /user_id TEXT NOT NULL REFERENCES users\(id\) ON DELETE CASCADE/);
  assert.match(dbSource, /price_ids JSONB NOT NULL/);
  assert.match(dbSource, /qualifies_pro BOOLEAN NOT NULL DEFAULT FALSE/);
  assert.match(dbSource, /event_created_at BIGINT NOT NULL DEFAULT 0/);
});

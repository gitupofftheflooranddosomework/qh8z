import test from 'node:test';
import assert from 'node:assert/strict';
import { generateShortCode } from '../src/shlink.mjs';

test('generated short codes stay compact, alphanumeric, and high-entropy', () => {
  const seen = new Set();
  for (let i = 0; i < 1000; i += 1) {
    const code = generateShortCode();
    assert.match(code, /^[A-Za-z0-9]{7}$/);
    seen.add(code);
  }
  assert.ok(seen.size > 990, `unexpected collision rate: ${seen.size}/1000 unique`);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeEmail, validEmail, validPassword, normalizeHttpUrl, normalizeSlug } from '../src/validation.mjs';

test('normalizes email', () => assert.equal(normalizeEmail(' Mark@Example.COM '), 'mark@example.com'));
test('validates email', () => { assert.equal(validEmail('a@example.com'), true); assert.equal(validEmail('nope'), false); });
test('requires useful password length', () => { assert.equal(validPassword('1234567890'), true); assert.equal(validPassword('short'), false); assert.equal(validPassword('x'.repeat(73)), false); });
test('only allows http URLs', () => { assert.equal(normalizeHttpUrl('https://example.com/x'), 'https://example.com/x'); assert.throws(() => normalizeHttpUrl('javascript:alert(1)')); });
test('validates custom slugs and reserved words', () => { assert.equal(normalizeSlug('Hello_123'), 'Hello_123'); assert.throws(() => normalizeSlug('api')); assert.throws(() => normalizeSlug('x')); });

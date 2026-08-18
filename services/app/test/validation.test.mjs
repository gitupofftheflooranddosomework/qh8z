import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeEmail, validEmail, validPassword, normalizeHttpUrl, normalizeSlug, accepted } from '../src/validation.mjs';

test('normalizes email', () => assert.equal(normalizeEmail(' Mark@Example.COM '), 'mark@example.com'));
test('validates email', () => { assert.equal(validEmail('a@example.com'), true); assert.equal(validEmail('nope'), false); });
test('requires useful password length', () => { assert.equal(validPassword('1234567890'), true); assert.equal(validPassword('short'), false); assert.equal(validPassword('x'.repeat(73)), false); });
test('only allows http URLs without embedded credentials', () => { assert.equal(normalizeHttpUrl('https://example.com/x'), 'https://example.com/x'); assert.throws(() => normalizeHttpUrl('javascript:alert(1)')); assert.throws(() => normalizeHttpUrl('https://user:pass@example.com')); });
test('rejects unreasonably large URLs', () => assert.throws(() => normalizeHttpUrl(`https://example.com/${'x'.repeat(8200)}`)));
test('validates custom slugs and reserved words', () => { assert.equal(normalizeSlug('Hello_123'), 'Hello_123'); for (const reserved of ['api','reset','rest']) assert.throws(() => normalizeSlug(reserved)); assert.throws(() => normalizeSlug('x')); });
test('normalizes checkbox acceptance', () => { assert.equal(accepted(true), true); assert.equal(accepted('on'), true); assert.equal(accepted('false'), false); });

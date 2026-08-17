import test from 'node:test';
import assert from 'node:assert/strict';
import { assertDestinationAllowed } from '../src/destination.mjs';

test('allows ordinary public destinations', () => assert.equal(assertDestinationAllowed('https://example.com/path'), 'https://example.com/path'));
test('blocks localhost and local names', () => { assert.throws(() => assertDestinationAllowed('http://localhost/x')); assert.throws(() => assertDestinationAllowed('http://printer.local/x')); });
test('blocks private and link-local IPv4 destinations', () => { for (const ip of ['127.0.0.1','10.1.2.3','172.16.1.1','192.168.1.1','169.254.169.254']) assert.throws(() => assertDestinationAllowed(`http://${ip}/`)); });
test('blocks loopback and unique-local IPv6 destinations', () => { assert.throws(() => assertDestinationAllowed('http://[::1]/')); assert.throws(() => assertDestinationAllowed('http://[fd00::1]/')); });

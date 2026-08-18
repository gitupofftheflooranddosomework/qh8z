import test from 'node:test';
import assert from 'node:assert/strict';
import { assertDestinationAllowed } from '../src/destination.mjs';

test('allows ordinary public destinations', () => assert.equal(assertDestinationAllowed('https://example.com/path'), 'https://example.com/path'));
test('blocks localhost, trailing-dot, single-label, and internal names', () => {
  for (const url of ['http://localhost/x','http://localhost./x','http://printer/x','http://printer.local/x','http://router.internal/x','http://device.home.arpa/x','http://service.test/x']) assert.throws(() => assertDestinationAllowed(url));
});
test('blocks private, link-local, and reserved IPv4 destinations', () => { for (const ip of ['127.0.0.1','10.1.2.3','172.16.1.1','192.168.1.1','169.254.169.254','192.0.2.1','198.51.100.1','203.0.113.1']) assert.throws(() => assertDestinationAllowed(`http://${ip}/`)); });
test('blocks loopback and unique-local IPv6 destinations', () => { assert.throws(() => assertDestinationAllowed('http://[::1]/')); assert.throws(() => assertDestinationAllowed('http://[fd00::1]/')); });
test('blocks IPv4-mapped IPv6 literals that can hide private IPv4 targets', () => { assert.throws(() => assertDestinationAllowed('http://[::ffff:127.0.0.1]/')); assert.throws(() => assertDestinationAllowed('http://[::ffff:7f00:1]/')); });

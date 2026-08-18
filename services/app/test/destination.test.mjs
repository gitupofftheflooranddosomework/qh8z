import test from 'node:test';
import assert from 'node:assert/strict';
import { assertDestinationAllowed, assertResolvedDestinationAllowed } from '../src/destination.mjs';

test('allows ordinary public destinations', () => assert.equal(assertDestinationAllowed('https://example.com/path'), 'https://example.com/path'));
test('blocks localhost, trailing-dot, single-label, and internal names', () => {
  for (const url of ['http://localhost/x','http://localhost./x','http://printer/x','http://printer.local/x','http://router.internal/x','http://device.home.arpa/x','http://service.test/x']) assert.throws(() => assertDestinationAllowed(url));
});
test('blocks private, link-local, and reserved IPv4 destinations', () => { for (const ip of ['127.0.0.1','10.1.2.3','172.16.1.1','192.168.1.1','169.254.169.254','192.0.2.1','198.51.100.1','203.0.113.1']) assert.throws(() => assertDestinationAllowed(`http://${ip}/`)); });
test('blocks loopback and unique-local IPv6 destinations', () => { assert.throws(() => assertDestinationAllowed('http://[::1]/')); assert.throws(() => assertDestinationAllowed('http://[fd00::1]/')); });
test('blocks IPv4-mapped IPv6 literals that can hide private IPv4 targets', () => { assert.throws(() => assertDestinationAllowed('http://[::ffff:127.0.0.1]/')); assert.throws(() => assertDestinationAllowed('http://[::ffff:7f00:1]/')); });

test('resolved destination check accepts hostnames whose complete address set is public', async () => {
  const lookup = async (hostname, options) => {
    assert.equal(hostname, 'public.testhost.com');
    assert.deepEqual(options, { all: true, verbatim: true });
    return [{ address: '93.184.216.34', family: 4 }, { address: '2606:2800:220:1:248:1893:25c8:1946', family: 6 }];
  };
  assert.equal(await assertResolvedDestinationAllowed('https://public.testhost.com/path', lookup), 'https://public.testhost.com/path');
});

test('resolved destination check blocks any hostname answer that reaches a private or reserved network', async () => {
  for (const address of ['127.0.0.1', '10.2.3.4', '169.254.169.254', 'fd00::1', 'fe80::1']) {
    const lookup = async () => [{ address, family: address.includes(':') ? 6 : 4 }];
    await assert.rejects(
      assertResolvedDestinationAllowed('https://public.testhost.com/', lookup),
      error => error?.code === 'unsafe_destination' && error?.status === 422 && error?.threats?.includes('PRIVATE_NETWORK')
    );
  }
});

test('resolved destination check fails closed when DNS is unavailable or malformed', async () => {
  await assert.rejects(
    assertResolvedDestinationAllowed('https://public.testhost.com/', async () => { throw new Error('resolver down'); }),
    error => error?.code === 'destination_resolution_unavailable' && error?.status === 503
  );
  await assert.rejects(
    assertResolvedDestinationAllowed('https://public.testhost.com/', async () => []),
    error => error?.code === 'destination_resolution_unavailable' && error?.status === 503
  );
});

test('literal public IP destinations do not perform DNS lookup', async () => {
  let called = false;
  const result = await assertResolvedDestinationAllowed('https://8.8.8.8/', async () => { called = true; return []; });
  assert.equal(result, 'https://8.8.8.8/');
  assert.equal(called, false);
});

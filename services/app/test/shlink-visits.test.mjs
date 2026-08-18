import test from 'node:test';
import assert from 'node:assert/strict';

process.env.SHLINK_API_KEY = 'test-shlink-api-key';
process.env.SHLINK_BASE_URL = 'http://shlink.test';

const originalFetch = globalThis.fetch;
const { getVisits } = await import('../src/shlink.mjs');

test.after(() => { globalThis.fetch = originalFetch; });

test('disabled or otherwise absent upstream links expose empty visit history', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ title: 'Not found' }), {
    status: 404,
    headers: { 'content-type': 'application/json' },
  });
  const result = await getVisits('disabled-link', 'Infinity', '1.5');
  assert.deepEqual(result.visits.data, []);
  assert.equal(result.visits.pagination.currentPage, 1);
  assert.equal(result.visits.pagination.itemsPerPage, 50);
  assert.equal(result.visits.pagination.totalItems, 0);
});

test('visit pagination clamps valid integers and passes successful responses through', async () => {
  let requestedUrl = '';
  globalThis.fetch = async url => {
    requestedUrl = String(url);
    return new Response(JSON.stringify({ visits: { data: [{ date: '2026-08-18T00:00:00Z' }] } }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  };
  const result = await getVisits('active-link', '2', '500');
  assert.equal(result.visits.data.length, 1);
  assert.match(requestedUrl, /page=2/);
  assert.match(requestedUrl, /itemsPerPage=100/);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { runHousekeepingTasks } from '../src/housekeeping.mjs';

test('housekeeping executes maintenance tasks sequentially', async () => {
  const order = [];
  const result = await runHousekeepingTasks([
    async () => { order.push('sessions'); },
    async () => { order.push('tokens'); },
    async () => { order.push('retention'); },
  ]);
  assert.equal(result, true);
  assert.deepEqual(order, ['sessions', 'tokens', 'retention']);
});

test('a second housekeeping pass is skipped while one is already running', async () => {
  let release;
  const first = runHousekeepingTasks([() => new Promise(resolve => { release = resolve; })]);
  await new Promise(resolve => setImmediate(resolve));
  const second = await runHousekeepingTasks([async () => { throw new Error('must not run'); }]);
  assert.equal(second, false);
  release();
  assert.equal(await first, true);
});

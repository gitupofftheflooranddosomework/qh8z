import { cleanupExpiredSessions, cleanupExpiredAuthTokens, cleanupRetainedOperationalData } from './db.mjs';

const DEFAULT_INTERVAL_MS = 6 * 60 * 60_000;
const DEFAULT_INITIAL_DELAY_MS = 15 * 60_000;

let timer = null;
let running = false;
let stopped = true;

export async function runHousekeepingTasks(tasks = [cleanupExpiredSessions, cleanupExpiredAuthTokens, cleanupRetainedOperationalData]) {
  if (running) return false;
  running = true;
  try {
    for (const task of tasks) await task();
    return true;
  } finally {
    running = false;
  }
}

function schedule(delayMs, intervalMs) {
  if (stopped) return;
  timer = setTimeout(async () => {
    try {
      await runHousekeepingTasks();
    } catch (error) {
      console.error(JSON.stringify({ level: 'error', event: 'housekeeping.failed', message: error.message }));
    } finally {
      schedule(intervalMs, intervalMs);
    }
  }, Math.max(1, delayMs));
  timer.unref();
}

export function startHousekeeping({ initialDelayMs = DEFAULT_INITIAL_DELAY_MS, intervalMs = DEFAULT_INTERVAL_MS } = {}) {
  if (!stopped) return;
  stopped = false;
  schedule(initialDelayMs, intervalMs);
}

export async function stopHousekeeping(waitMs = 5000) {
  stopped = true;
  if (timer) clearTimeout(timer);
  timer = null;
  const deadline = Date.now() + Math.max(0, waitMs);
  while (running && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 50));
  return !running;
}

import http from 'node:http';
import { config, startupProblems } from './config.mjs';
import { pool } from './db.mjs';
import { startHousekeeping, stopHousekeeping } from './housekeeping.mjs';
import { startReputationWorker, stopReputationWorker } from './reputation.mjs';
import { validateAdminMfaEncryption } from './startup-security.mjs';

let server = null;
let shuttingDown = false;
const originalListen = http.Server.prototype.listen;

// Capture the Express-created server without coupling the routing module to
// process lifecycle concerns. This keeps startup/tests simple while allowing
// Docker SIGTERM to drain in-flight HTTP requests before PostgreSQL closes.
http.Server.prototype.listen = function qh8zCapturedListen(...args) {
  server = this;
  return originalListen.apply(this, args);
};

async function shutdown(signal, requestedExitCode = 0) {
  if (shuttingDown) return;
  shuttingDown = true;
  console.log(JSON.stringify({ level: 'info', event: 'app.shutdown_started', signal }));

  const forceTimer = setTimeout(() => {
    console.error(JSON.stringify({ level: 'error', event: 'app.shutdown_forced', signal }));
    try { server?.closeAllConnections?.(); } catch {}
    process.exit(1);
  }, 15_000);

  try {
    const httpDrain = new Promise(resolve => {
      if (!server) return resolve(true);
      server.close(error => resolve(!error));
      try { server.closeIdleConnections?.(); } catch {}
    });
    const workerDrain = stopReputationWorker(5000);
    const housekeepingDrain = stopHousekeeping(5000);
    const [httpResult, workerResult, housekeepingResult] = await Promise.allSettled([httpDrain, workerDrain, housekeepingDrain]);

    if (httpResult.status !== 'fulfilled' || httpResult.value !== true) {
      console.error(JSON.stringify({ level: 'error', event: 'app.shutdown_http_drain_failed' }));
    }
    if (workerResult.status !== 'fulfilled' || workerResult.value !== true) {
      console.warn(JSON.stringify({ level: 'warn', event: 'app.shutdown_worker_drain_incomplete' }));
    }
    if (housekeepingResult.status !== 'fulfilled' || housekeepingResult.value !== true) {
      console.warn(JSON.stringify({ level: 'warn', event: 'app.shutdown_housekeeping_drain_incomplete' }));
    }

    await pool.end();
    clearTimeout(forceTimer);
    console.log(JSON.stringify({ level: 'info', event: 'app.shutdown_completed', signal }));
    const drainFailed = httpResult.status !== 'fulfilled' || httpResult.value !== true;
    process.exit(requestedExitCode || drainFailed ? 1 : 0);
  } catch (error) {
    clearTimeout(forceTimer);
    console.error(JSON.stringify({ level: 'error', event: 'app.shutdown_failed', signal, message: error.message }));
    process.exit(1);
  }
}

process.on('SIGTERM', () => void shutdown('SIGTERM'));
process.on('SIGINT', () => void shutdown('SIGINT'));

try {
  const problems = startupProblems();
  if (problems.length) throw new Error(`QH8Z startup blocked: ${problems.join('; ')}`);
  await import('./server.mjs');
  http.Server.prototype.listen = originalListen;

  if (config.publicLaunchMode) {
    const mfaProblems = await validateAdminMfaEncryption();
    if (mfaProblems.length) throw new Error(`QH8Z startup blocked: ${mfaProblems.join('; ')}`);
  }

  startHousekeeping();
  startReputationWorker();
} catch (error) {
  http.Server.prototype.listen = originalListen;
  console.error(JSON.stringify({ level: 'error', event: 'app.startup_failed', message: error.message, stack: process.env.NODE_ENV === 'production' ? undefined : error.stack }));
  if (server) {
    await shutdown('STARTUP_FAILURE', 1);
  } else {
    try { await pool.end(); } catch {}
    process.exitCode = 1;
  }
}

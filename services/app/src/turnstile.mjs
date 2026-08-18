import crypto from 'node:crypto';
import { config } from './config.mjs';

function expectedHostname() {
  try { return new URL(config.appBaseUrl).hostname; } catch { return null; }
}

function unavailable() {
  const error = new Error('Human verification is temporarily unavailable');
  error.status = 503;
  return error;
}

export async function verifyTurnstile(token, action, remoteIp) {
  if (!config.turnstileSecretKey) {
    if (config.turnstileRequired) throw unavailable();
    return { success: true, bypassed: true };
  }
  if (!token || String(token).length > 2048) {
    const error = new Error('Please complete the human verification challenge');
    error.status = 400;
    throw error;
  }
  const body = new URLSearchParams({
    secret: config.turnstileSecretKey,
    response: String(token),
    idempotency_key: crypto.randomUUID(),
  });
  if (remoteIp) body.set('remoteip', remoteIp);

  let response;
  try {
    response = await fetch('https://challenges.cloudflare.com/turnstile/v0/siteverify', {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded', accept: 'application/json' },
      body,
      signal: AbortSignal.timeout(6000),
    });
  } catch {
    throw unavailable();
  }
  if (!response.ok) throw unavailable();

  let result;
  try {
    result = await response.json();
  } catch {
    throw unavailable();
  }
  if (!result || typeof result !== 'object' || Array.isArray(result)) throw unavailable();

  const hostname = expectedHostname();
  if (!result.success || (action && result.action !== action) || (config.env === 'production' && hostname && result.hostname !== hostname)) {
    const error = new Error('Human verification failed. Please try again.');
    error.status = 400;
    throw error;
  }
  return result;
}

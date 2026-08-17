import crypto from 'node:crypto';
import { config } from './config.mjs';

function expectedHostname() {
  try { return new URL(config.appBaseUrl).hostname; } catch { return null; }
}

export async function verifyTurnstile(token, action, remoteIp) {
  if (!config.turnstileSecretKey) {
    if (config.turnstileRequired) {
      const error = new Error('Human verification is temporarily unavailable');
      error.status = 503;
      throw error;
    }
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
    const error = new Error('Human verification is temporarily unavailable');
    error.status = 503;
    throw error;
  }
  if (!response.ok) {
    const error = new Error('Human verification is temporarily unavailable');
    error.status = 503;
    throw error;
  }
  const result = await response.json();
  const hostname = expectedHostname();
  if (!result.success || (action && result.action !== action) || (config.env === 'production' && hostname && result.hostname !== hostname)) {
    const error = new Error('Human verification failed. Please try again.');
    error.status = 400;
    throw error;
  }
  return result;
}

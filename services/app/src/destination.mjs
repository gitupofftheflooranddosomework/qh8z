import net from 'node:net';
import { lookup as dnsLookup } from 'node:dns/promises';
import { config } from './config.mjs';

const LOCAL_SUFFIXES = ['.localhost', '.local', '.internal', '.lan', '.home', '.home.arpa', '.localdomain', '.test', '.invalid', '.example', '.corp'];
const DNS_TIMEOUT_MS = 3000;

function normalizeHostname(value) {
  return String(value || '').replace(/^\[|\]$/g, '').replace(/\.$/, '').toLowerCase();
}

function blockedIpv4(ip) {
  const parts = ip.split('.').map(Number);
  const [a,b,c] = parts;
  return a === 0 || a === 10 || a === 127 || a >= 224 ||
    (a === 100 && b >= 64 && b <= 127) ||
    (a === 169 && b === 254) ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    (a === 192 && b === 0 && (c === 0 || c === 2)) ||
    (a === 198 && (b === 18 || b === 19 || (b === 51 && c === 100))) ||
    (a === 203 && b === 0 && c === 113);
}

function blockedIpv6(ip) {
  const lower = ip.toLowerCase();
  if (lower === '::' || lower === '::1' || lower.startsWith('fc') || lower.startsWith('fd') || lower.startsWith('fe8') || lower.startsWith('fe9') || lower.startsWith('fea') || lower.startsWith('feb') || lower.startsWith('ff')) return true;
  if (lower.startsWith('::ffff:')) return true;
  return false;
}

function blockedHostname(hostname) {
  if (!hostname.includes('.')) return true;
  return hostname === 'localhost' || LOCAL_SUFFIXES.some(suffix => hostname === suffix.slice(1) || hostname.endsWith(suffix));
}

function blockedAddress(address) {
  const family = net.isIP(address);
  return (family === 4 && blockedIpv4(address)) || (family === 6 && blockedIpv6(address));
}

function policyError(message, threats = ['PRIVATE_NETWORK']) {
  const error = new Error(message);
  error.status = 422;
  error.code = 'unsafe_destination';
  error.threats = threats;
  return error;
}

function unsafeResolvedDestination(address) {
  const error = policyError('Private or reserved network destinations are not allowed, including hostnames that resolve to private addresses');
  error.address = address;
  return error;
}

export function assertDestinationAllowed(url) {
  const parsed = new URL(url);
  const hostname = normalizeHostname(parsed.hostname);
  const shortHost = (() => { try { return normalizeHostname(new URL(config.publicShortBaseUrl).hostname); } catch { return ''; } })();
  if (hostname === shortHost || hostname === `www.${shortHost}`) throw policyError('QH8Z links cannot redirect back into the QH8Z short domain', ['SELF_REDIRECT']);
  const family = net.isIP(hostname);
  if (!family && blockedHostname(hostname)) throw policyError('Local-network destinations are not allowed');
  if (family && blockedAddress(hostname)) throw policyError('Private or reserved network destinations are not allowed');
  return parsed.toString();
}

export async function assertResolvedDestinationAllowed(url, lookup = dnsLookup) {
  const normalized = assertDestinationAllowed(url);
  const parsed = new URL(normalized);
  const hostname = normalizeHostname(parsed.hostname);
  if (net.isIP(hostname)) return normalized;

  let timer;
  let addresses;
  try {
    addresses = await Promise.race([
      lookup(hostname, { all: true, verbatim: true }),
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error('DNS lookup timed out')), DNS_TIMEOUT_MS);
        timer.unref?.();
      }),
    ]);
  } catch (cause) {
    const error = new Error('Destination hostname could not be safely resolved');
    error.status = 503;
    error.code = 'destination_resolution_unavailable';
    error.cause = cause;
    throw error;
  } finally {
    if (timer) clearTimeout(timer);
  }

  if (!Array.isArray(addresses) || addresses.length === 0) {
    const error = new Error('Destination hostname did not resolve to a usable address');
    error.status = 503;
    error.code = 'destination_resolution_unavailable';
    throw error;
  }

  for (const entry of addresses) {
    const address = normalizeHostname(entry?.address);
    if (!address || !net.isIP(address)) {
      const error = new Error('Destination hostname returned an invalid DNS address');
      error.status = 503;
      error.code = 'destination_resolution_unavailable';
      throw error;
    }
    if (blockedAddress(address)) throw unsafeResolvedDestination(address);
  }

  return normalized;
}

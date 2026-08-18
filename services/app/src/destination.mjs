import net from 'node:net';
import { config } from './config.mjs';

const LOCAL_SUFFIXES = ['.localhost', '.local', '.internal', '.lan', '.home', '.home.arpa', '.localdomain', '.test', '.invalid', '.example', '.corp'];

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
  // Reject IPv4-mapped IPv6 literals entirely. WHATWG URL canonicalization can
  // turn 127.0.0.1 into ::ffff:7f00:1, which is easy to mis-parse as public.
  if (lower.startsWith('::ffff:')) return true;
  return false;
}

function blockedHostname(hostname) {
  if (!hostname.includes('.')) return true;
  return hostname === 'localhost' || LOCAL_SUFFIXES.some(suffix => hostname === suffix.slice(1) || hostname.endsWith(suffix));
}

export function assertDestinationAllowed(url) {
  const parsed = new URL(url);
  const hostname = normalizeHostname(parsed.hostname);
  const shortHost = (() => { try { return normalizeHostname(new URL(config.publicShortBaseUrl).hostname); } catch { return ''; } })();
  if (hostname === shortHost || hostname === `www.${shortHost}`) throw new Error('QH8Z links cannot redirect back into the QH8Z short domain');
  const family = net.isIP(hostname);
  if (!family && blockedHostname(hostname)) throw new Error('Local-network destinations are not allowed');
  if ((family === 4 && blockedIpv4(hostname)) || (family === 6 && blockedIpv6(hostname))) throw new Error('Private or reserved network destinations are not allowed');
  return parsed.toString();
}

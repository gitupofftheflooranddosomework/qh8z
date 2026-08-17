import net from 'node:net';
import { config } from './config.mjs';

function blockedIpv4(ip) {
  const parts = ip.split('.').map(Number);
  const [a,b] = parts;
  return a === 0 || a === 10 || a === 127 || a >= 224 ||
    (a === 100 && b >= 64 && b <= 127) ||
    (a === 169 && b === 254) ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    (a === 198 && (b === 18 || b === 19));
}

function blockedIpv6(ip) {
  const lower = ip.toLowerCase();
  if (lower === '::' || lower === '::1' || lower.startsWith('fc') || lower.startsWith('fd') || lower.startsWith('fe8') || lower.startsWith('fe9') || lower.startsWith('fea') || lower.startsWith('feb') || lower.startsWith('ff')) return true;
  if (lower.startsWith('::ffff:')) {
    const mapped = lower.slice(7);
    return net.isIP(mapped) === 4 && blockedIpv4(mapped);
  }
  return false;
}

export function assertDestinationAllowed(url) {
  const parsed = new URL(url);
  const hostname = parsed.hostname.replace(/^\[|\]$/g, '').toLowerCase();
  const shortHost = (() => { try { return new URL(config.publicShortBaseUrl).hostname.toLowerCase(); } catch { return ''; } })();
  if (hostname === shortHost || hostname === `www.${shortHost}`) throw new Error('QH8Z links cannot redirect back into the QH8Z short domain');
  if (hostname === 'localhost' || hostname.endsWith('.localhost') || hostname.endsWith('.local')) throw new Error('Local-network destinations are not allowed');
  const family = net.isIP(hostname);
  if ((family === 4 && blockedIpv4(hostname)) || (family === 6 && blockedIpv6(hostname))) throw new Error('Private or reserved network destinations are not allowed');
  return parsed.toString();
}

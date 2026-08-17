import http from 'node:http';
import { URL } from 'node:url';

const port = Number(process.env.PORT || 3000);
const adminToken = process.env.QH8Z_ADMIN_TOKEN || '';
const shlinkBaseUrl = (process.env.SHLINK_BASE_URL || 'http://shlink:8080').replace(/\/$/, '');
const shlinkApiKey = process.env.SHLINK_API_KEY || '';
const publicShortBaseUrl = (process.env.PUBLIC_SHORT_BASE_URL || 'http://localhost:8080').replace(/\/$/, '');

function send(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    'content-type': 'application/json; charset=utf-8',
    'content-length': Buffer.byteLength(payload),
  });
  res.end(payload);
}

async function readJson(req) {
  const chunks = [];
  let size = 0;

  for await (const chunk of req) {
    size += chunk.length;
    if (size > 64 * 1024) throw new Error('Request body too large');
    chunks.push(chunk);
  }

  if (chunks.length === 0) return {};
  return JSON.parse(Buffer.concat(chunks).toString('utf8'));
}

function authorized(req) {
  if (!adminToken) return false;
  return req.headers.authorization === `Bearer ${adminToken}`;
}

function normalizeUrl(value) {
  const parsed = new URL(value);
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new Error('Only http and https destinations are allowed');
  }
  return parsed.toString();
}

const server = http.createServer(async (req, res) => {
  try {
    const requestUrl = new URL(req.url || '/', `http://${req.headers.host || 'localhost'}`);

    if (req.method === 'GET' && requestUrl.pathname === '/healthz') {
      return send(res, 200, {
        ok: true,
        service: 'qh8z-gateway',
        shlinkConfigured: Boolean(shlinkApiKey),
      });
    }

    if (req.method === 'POST' && requestUrl.pathname === '/api/links') {
      if (!authorized(req)) return send(res, 401, { error: 'unauthorized' });
      if (!shlinkApiKey) return send(res, 503, { error: 'shlink_api_key_not_configured' });

      const body = await readJson(req);
      if (typeof body.longUrl !== 'string' || !body.longUrl.trim()) {
        return send(res, 400, { error: 'longUrl is required' });
      }

      const longUrl = normalizeUrl(body.longUrl.trim());
      const customSlug = typeof body.customSlug === 'string' && body.customSlug.trim()
        ? body.customSlug.trim()
        : undefined;

      const shlinkPayload = {
        longUrl,
        ...(customSlug ? { customSlug } : {}),
      };

      const upstream = await fetch(`${shlinkBaseUrl}/rest/v3/short-urls`, {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          'X-Api-Key': shlinkApiKey,
        },
        body: JSON.stringify(shlinkPayload),
      });

      const text = await upstream.text();
      let upstreamBody;
      try {
        upstreamBody = JSON.parse(text);
      } catch {
        upstreamBody = { raw: text };
      }

      if (!upstream.ok) {
        return send(res, upstream.status, {
          error: 'shlink_error',
          upstream: upstreamBody,
        });
      }

      const shortCode = upstreamBody.shortCode;
      return send(res, 201, {
        id: shortCode,
        shortCode,
        shortUrl: shortCode ? `${publicShortBaseUrl}/${shortCode}` : upstreamBody.shortUrl,
        longUrl: upstreamBody.longUrl || longUrl,
      });
    }

    return send(res, 404, { error: 'not_found' });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    const status = message.includes('URL') || message.includes('JSON') || message.includes('body') ? 400 : 500;
    return send(res, status, { error: 'request_failed', message });
  }
});

server.listen(port, '0.0.0.0', () => {
  console.log(`QH8Z gateway listening on :${port}`);
});

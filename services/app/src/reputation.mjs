import { config } from './config.mjs';

const THREAT_TYPES = ['MALWARE', 'SOCIAL_ENGINEERING', 'UNWANTED_SOFTWARE'];

export async function checkUrlReputation(url) {
  if (!config.webRiskApiKey) {
    if (config.webRiskRequired) {
      const error = new Error('URL reputation checking is required but WEB_RISK_API_KEY is not configured');
      error.status = 503;
      throw error;
    }
    return { checked: false, threats: [] };
  }

  const params = new URLSearchParams({ uri: url, key: config.webRiskApiKey });
  for (const threatType of THREAT_TYPES) params.append('threatTypes', threatType);

  try {
    const response = await fetch(`https://webrisk.googleapis.com/v1/uris:search?${params}`, {
      method: 'GET',
      signal: AbortSignal.timeout(5000),
      headers: { accept: 'application/json' },
    });

    if (!response.ok) {
      const error = new Error(`Web Risk returned ${response.status}`);
      error.status = 503;
      throw error;
    }

    const body = await response.json();
    return { checked: true, threats: body?.threat?.threatTypes || [] };
  } catch (error) {
    if (config.webRiskRequired) {
      error.status = 503;
      throw error;
    }
    console.warn('URL reputation check unavailable:', error.message);
    return { checked: false, threats: [] };
  }
}

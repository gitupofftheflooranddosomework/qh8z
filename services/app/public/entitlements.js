const PRO_FIELDS = ['expiresAt', 'maxVisits'];

function currentPlanIsPro() {
  return document.getElementById('planBadge')?.textContent?.trim().toUpperCase() === 'PRO';
}

function syncProControls() {
  const pro = currentPlanIsPro();
  for (const formId of ['createLinkForm', 'editForm']) {
    const form = document.getElementById(formId);
    if (!form) continue;
    for (const name of PRO_FIELDS) {
      const input = form.elements.namedItem(name);
      if (!(input instanceof HTMLInputElement)) continue;
      input.disabled = !pro;
      input.title = pro ? '' : 'QH8Z Pro control. Existing saved values are preserved after a downgrade.';
      input.closest('label')?.classList.toggle('pro-control-locked', !pro);
    }
  }
}

function parseCsvLine(line) {
  const fields = [];
  let field = '';
  let quoted = false;
  for (let i = 0; i < line.length; i += 1) {
    const char = line[i];
    if (char === '"') {
      if (quoted && line[i + 1] === '"') { field += '"'; i += 1; }
      else quoted = !quoted;
      continue;
    }
    if (char === ',' && !quoted) { fields.push(field.trim()); field = ''; continue; }
    field += char;
  }
  if (quoted) throw new Error('A bulk CSV row has an unclosed quote.');
  fields.push(field.trim());
  return fields;
}

function isCompleteHttpUrl(value) {
  try {
    const parsed = new URL(String(value || '').trim());
    return ['http:', 'https:'].includes(parsed.protocol) && Boolean(parsed.hostname);
  } catch { return false; }
}

function parseBulkLine(line) {
  const trimmed = String(line || '').trim();
  if (!trimmed) return null;

  // A plain URL wins before CSV parsing. This keeps valid paths/query strings
  // containing commas intact instead of treating their comma as an alias split.
  if (isCompleteHttpUrl(trimmed)) return { longUrl: trimmed };

  const fields = trimmed.includes('\t') ? trimmed.split('\t').map(value => value.trim()) : parseCsvLine(trimmed);
  const [longUrl = '', customSlug = '', ...titleParts] = fields;
  if (!isCompleteHttpUrl(longUrl)) throw new Error(`Invalid bulk destination: ${longUrl || '(blank)'}`);
  return {
    longUrl,
    ...(customSlug ? { customSlug } : {}),
    ...(titleParts.length && titleParts.join(',').trim() ? { title: titleParts.join(',').trim() } : {}),
  };
}

function parseBulkRows(text) {
  const lines = String(text || '').split(/\r?\n/).filter(line => line.trim());
  if (!lines.length) throw new Error('Paste at least one destination.');
  if (lines.length > 100) throw new Error('Bulk creation is limited to 100 links per request.');
  return lines.map(parseBulkLine).filter(Boolean);
}

async function submitBulk(event) {
  const button = event.target.closest('#bulkSubmitBtn');
  if (!button) return;
  event.preventDefault();
  event.stopImmediatePropagation();

  const input = document.getElementById('bulkInput');
  const status = document.getElementById('bulkStatus');
  const resultsRoot = document.getElementById('bulkResults');
  if (!input || !status || !resultsRoot) return;

  let links;
  try { links = parseBulkRows(input.value); }
  catch (error) { status.textContent = error.message; return; }

  button.disabled = true;
  status.textContent = `Creating ${links.length} link${links.length === 1 ? '' : 's'}…`;
  resultsRoot.innerHTML = '';
  try {
    const response = await fetch('/api/links/bulk', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ links }),
    });
    const text = await response.text();
    let payload = null;
    try { payload = text ? JSON.parse(text) : null; } catch { payload = null; }
    if (!response.ok) throw new Error(payload?.message || payload?.error || `Bulk creation failed (${response.status}).`);

    status.textContent = `${payload.created || 0} created · ${payload.failed || 0} failed`;
    resultsRoot.innerHTML = (payload.results || []).map((item, index) => item.ok
      ? `<div class="bulk-result ok"><span>${index + 1}</span><a href="${escapeAttribute(item.link?.short_url)}" target="_blank" rel="noreferrer">${escapeText(item.link?.short_url)}</a></div>`
      : `<div class="bulk-result failed"><span>${index + 1}</span><strong>${escapeText(item.message || item.error || 'Create failed')}</strong></div>`).join('');
    if (payload.created) document.getElementById('refreshBtn')?.click();
  } catch (error) {
    status.textContent = error.message;
  } finally {
    button.disabled = false;
  }
}

function escapeText(value) {
  return String(value ?? '').replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
}

function escapeAttribute(value) {
  return escapeText(value);
}

document.addEventListener('DOMContentLoaded', () => {
  syncProControls();
  const badge = document.getElementById('planBadge');
  if (badge) new MutationObserver(syncProControls).observe(badge, { childList: true, characterData: true, subtree: true });

  const help = document.querySelector('#bulkDialog p');
  if (help) help.innerHTML = 'Paste one URL per line. For alias/title, use <code>URL,alias,title</code> or tab-separated fields. Quote CSV fields that contain commas.';
});

// The edit controller populates values after the click. Re-apply the lock on
// the next task so a former Pro user sees the saved controls but FormData omits
// them from ordinary Free edits, allowing the backend to preserve those values.
document.addEventListener('click', event => {
  if (event.target.closest('[data-action="edit"]')) setTimeout(syncProControls, 0);
  if (event.target.closest('#bulkSubmitBtn')) void submitBulk(event);
}, true);

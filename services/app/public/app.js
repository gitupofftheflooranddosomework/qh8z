const $ = selector => document.querySelector(selector);
const $$ = selector => [...document.querySelectorAll(selector)];

const state = {
  config: null,
  user: null,
  links: [],
  total: 0,
  limit: 25,
  offset: 0,
  q: '',
  status: 'all',
  tag: '',
  stats: new Map(),
  widgets: new Map(),
  activeSection: 'linksSection',
  activeTotal: 0,
  currentDetail: null,
};

async function api(path, options = {}) {
  const headers = { ...(options.body && !(options.body instanceof FormData) ? { 'content-type': 'application/json' } : {}), ...(options.headers || {}) };
  const response = await fetch(path, { credentials: 'same-origin', ...options, headers });
  const text = await response.text();
  const data = text ? (() => { try { return JSON.parse(text); } catch { return { raw: text }; } })() : null;
  if (!response.ok) {
    const error = new Error(data?.message || data?.error || `Request failed (${response.status})`);
    error.status = response.status;
    error.data = data;
    throw error;
  }
  return data;
}

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
}

function flash(message, error = false) {
  const element = $('#flash');
  element.textContent = message;
  element.classList.remove('hidden', 'error');
  if (error) element.classList.add('error');
  clearTimeout(flash.timer);
  flash.timer = setTimeout(() => element.classList.add('hidden'), 6000);
}

function authError(message) {
  const element = $('#authNotice');
  element.textContent = message;
  element.classList.remove('hidden');
}

function formJson(form) {
  const data = Object.fromEntries(new FormData(form).entries());
  if ('acceptTerms' in data) data.acceptTerms = true;
  return data;
}

function humanDate(value) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : '—';
}

function toLocalInput(value) {
  if (!value) return '';
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return '';
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function statusLabel(link) {
  return ({ active: 'Active', disabled: 'Disabled', archived: 'Archived', expired: 'Expired' })[link.state] || link.state || 'Unknown';
}

function userEligible() {
  return Boolean(state.user && !state.user.suspended && (!state.config.emailVerificationRequired || state.user.emailVerified) && !state.user.mustAcceptTerms);
}

function isPro() {
  return state.user?.plan === 'pro';
}

function waitForTurnstile() {
  return new Promise((resolve, reject) => {
    let checks = 0;
    const timer = setInterval(() => {
      if (window.turnstile) { clearInterval(timer); resolve(window.turnstile); }
      else if (++checks > 50) { clearInterval(timer); reject(new Error('Human verification failed to load.')); }
    }, 100);
  });
}

async function renderTurnstile(action) {
  if (!state.config?.turnstileSiteKey) return;
  const element = document.getElementById(`${action}Turnstile`);
  if (!element || state.widgets.has(action)) return;
  try {
    const turnstile = await waitForTurnstile();
    const widget = turnstile.render(element, {
      sitekey: state.config.turnstileSiteKey,
      action,
      theme: 'dark',
      callback: token => { element.dataset.token = token; },
      'expired-callback': () => { element.dataset.token = ''; },
    });
    state.widgets.set(action, widget);
  } catch (error) { authError(error.message); }
}

function resetTurnstile(action) {
  const element = document.getElementById(`${action}Turnstile`);
  if (element) element.dataset.token = '';
  if (window.turnstile && state.widgets.has(action)) window.turnstile.reset(state.widgets.get(action));
}

function withTurnstile(data, action) {
  const element = document.getElementById(`${action}Turnstile`);
  if (state.config?.turnstileRequired || state.config?.turnstileSiteKey) data.turnstileToken = element?.dataset.token || '';
  return data;
}

function setAuthMode(mode) {
  const signup = mode === 'signup';
  $('#authNotice').classList.add('hidden');
  $('#mfaLoginForm').classList.add('hidden');
  $('#loginForm').classList.toggle('hidden', signup);
  $('#signupForm').classList.toggle('hidden', !signup);
  $('#authTitle').textContent = signup ? 'Make links smaller.' : 'Welcome back.';
  $('#authCopy').textContent = signup ? 'Create a QH8Z account and your first controlled short link.' : 'Sign in to manage every QH8Z link from one place.';
  $('#switchCopy').textContent = signup ? 'Already have an account?' : 'New to QH8Z?';
  $('#authSwitch').textContent = signup ? 'Sign in' : 'Create an account';
  location.hash = mode;
  renderTurnstile(signup ? 'signup' : 'login');
}

function updateEligibilityUi() {
  const eligible = userEligible();
  $('#verificationBanner').classList.toggle('hidden', !state.user || state.user.emailVerified || !state.config.emailVerificationRequired);
  $('#termsBanner').classList.toggle('hidden', !state.user?.mustAcceptTerms);
  $('#suspendedBanner').classList.toggle('hidden', !state.user?.suspended);
  $('#createLinkBtn').disabled = !eligible;
  $('#globalCreateBtn').disabled = !eligible;
  $('#createLocked').classList.toggle('hidden', eligible);
  $('#upgradeBtn').disabled = !eligible;
  $('#apiTokenForm').querySelector('button[type="submit"]').disabled = !eligible || !isPro();
  $('#apiTokenProNotice').classList.toggle('hidden', isPro());
}

function updateMfaUi() {
  if (!state.user) return;
  $('#mfaStatus').textContent = state.user.mfaEnabled
    ? 'Enabled — required at sign-in and for sensitive account changes.'
    : 'Not enabled. Add an authenticator for stronger account protection.';
  $('#mfaSetupBtn').classList.toggle('hidden', state.user.mfaEnabled);
  $('#mfaDisableBtn').classList.toggle('hidden', !state.user.mfaEnabled || state.user.isAdmin);
}

function showSection(id) {
  state.activeSection = id;
  $$('.dashboard-section').forEach(section => section.classList.toggle('hidden', section.id !== id));
  $$('.side-link[data-section]').forEach(button => button.classList.toggle('active', button.dataset.section === id));
  const titles = {
    linksSection: ['Your links', 'Create, measure, edit, and control every redirect.'],
    billingPanel: ['Plan & billing', 'Choose the capacity and automation level you need.'],
    developerPanel: ['Developer', 'Automate link management with scoped API tokens.'],
    accountPanel: ['Account', 'Identity, security, portability, and account controls.'],
    adminPanel: ['Trust & safety', 'Protect QH8Z, its users, and the qh8z.com domain.'],
  };
  const [title, subtitle] = titles[id] || titles.linksSection;
  $('#pageTitle').textContent = title;
  $('#pageSubtitle').textContent = subtitle;
  $('#globalCreateBtn').classList.toggle('hidden', id !== 'linksSection');
  if (id === 'developerPanel') loadApiTokens();
  if (id === 'adminPanel' && state.user?.isAdmin) { loadReports(); loadUsers(); }
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

async function refreshMe() {
  const me = await api('/api/me');
  state.user = me.user;
  if (state.user) {
    $('#userName').textContent = state.user.name;
    $('#userEmail').textContent = state.user.email;
    $('#planBadge').textContent = state.user.plan.toUpperCase();
    $('#planLimit').textContent = (state.config.plans[state.user.plan]?.links ?? 25).toLocaleString();
    $('#adminNav').classList.toggle('hidden', !state.user.isAdmin);
    $('#adminPanel').classList.toggle('hidden', !state.user.isAdmin && state.activeSection === 'adminPanel');
    $('#upgradeBtn').classList.toggle('hidden', state.user.plan === 'pro' || !state.config.billingEnabled);
    $('#portalBtn').classList.toggle('hidden', !state.config.billingEnabled);
    updateMfaUi();
    updateEligibilityUi();
  }
}

async function boot() {
  state.config = await api('/api/config');
  const me = await api('/api/me');
  state.user = me.user;
  if (!state.user) return setAuthMode(location.hash === '#signup' ? 'signup' : 'login');
  $('#authView').classList.add('hidden');
  $('#dashboard').classList.remove('hidden');
  await refreshMe();
  showSection('linksSection');
  await loadLinks();
  const billing = new URLSearchParams(location.search).get('billing');
  if (billing === 'success') flash('Pro activated. Welcome aboard.');
}

async function loadMetrics() {
  try {
    const active = await api('/api/links?status=active&limit=1');
    state.activeTotal = active.total || 0;
    $('#activeCount').textContent = state.activeTotal.toLocaleString();
  } catch { $('#activeCount').textContent = '—'; }
}

async function loadLinks() {
  const params = new URLSearchParams({ limit: String(state.limit), offset: String(state.offset), status: state.status });
  if (state.q) params.set('q', state.q);
  if (state.tag) params.set('tag', state.tag);
  $('#linkList').innerHTML = '<div class="empty">Loading your links…</div>';
  try {
    const data = await api(`/api/links?${params}`);
    state.links = data.links || [];
    state.total = data.total || 0;
    $('#resultCount').textContent = state.total.toLocaleString();
    $('#libraryMeta').textContent = `${state.total.toLocaleString()} link${state.total === 1 ? '' : 's'} in this view`;
    renderLinks();
    renderPagination(data);
    await loadMetrics();
    loadVisibleStats();
  } catch (error) {
    $('#linkList').innerHTML = `<div class="empty error-text">${escapeHtml(error.message)}</div>`;
  }
}

function renderPagination(data) {
  const start = state.total ? state.offset + 1 : 0;
  const end = Math.min(state.offset + state.links.length, state.total);
  $('#pageMeta').textContent = `${start.toLocaleString()}–${end.toLocaleString()} of ${state.total.toLocaleString()}`;
  $('#prevPageBtn').disabled = state.offset <= 0;
  $('#nextPageBtn').disabled = !data.hasMore;
}

function linkActions(link) {
  const common = `<button class="btn btn-sm" data-action="details">Details</button><button class="btn btn-sm" data-action="qr">QR</button>`;
  if (link.state === 'disabled') return `${common}<button class="btn btn-sm btn-primary" data-action="restore">Restore</button>`;
  const archive = link.state === 'archived' ? '<button class="btn btn-sm" data-action="unarchive">Unarchive</button>' : '<button class="btn btn-sm" data-action="archive">Archive</button>';
  const edit = userEligible() ? '<button class="btn btn-sm" data-action="edit">Edit</button>' : '';
  const disable = userEligible() ? '<button class="btn btn-sm btn-danger" data-action="disable">Disable</button>' : '';
  return `<button class="btn btn-sm" data-action="copy">Copy</button>${common}${edit}${archive}${disable}`;
}

function renderLinks() {
  const root = $('#linkList');
  if (!state.links.length) {
    root.innerHTML = '<div class="empty qh-empty"><div class="empty-orbit">Q8</div><strong>No links in this view.</strong><span>Create one above or clear your filters.</span></div>';
    $('#visitCount').textContent = '0';
    return;
  }
  root.innerHTML = state.links.map(link => {
    const stats = state.stats.get(link.id);
    const tags = (link.tags || []).map(tag => `<span class="tag-chip">${escapeHtml(tag)}</span>`).join('');
    const visits = stats ? `${Number(stats.visits?.total || 0).toLocaleString()} visits` : 'visits loading…';
    const meta = [humanDate(link.created_at), link.expires_at ? `expires ${humanDate(link.expires_at)}` : null, link.max_visits ? `max ${Number(link.max_visits).toLocaleString()} visits` : null].filter(Boolean).join(' · ');
    return `<article class="qh-link-card" data-id="${escapeHtml(link.id)}">
      <div class="link-main">
        <div class="link-title-row"><div><div class="link-title">${escapeHtml(link.title || link.short_code)}</div><a class="short-url" href="${escapeHtml(link.short_url)}" target="_blank" rel="noreferrer">${escapeHtml(link.short_url)}</a></div><span class="state-chip state-${escapeHtml(link.state)}">${escapeHtml(statusLabel(link))}</span></div>
        <div class="long-url" title="${escapeHtml(link.long_url)}">${escapeHtml(link.long_url)}</div>
        ${tags ? `<div class="tag-row">${tags}</div>` : ''}
        <div class="link-meta"><span>${escapeHtml(meta)}</span><span>${escapeHtml(visits)}</span></div>
      </div>
      <div class="link-actions">${linkActions(link)}</div>
    </article>`;
  }).join('');
}

async function loadVisibleStats() {
  const active = state.links.filter(link => link.state !== 'disabled').slice(0, 25);
  let total = 0;
  let loaded = 0;
  for (let offset = 0; offset < active.length; offset += 6) {
    const chunk = active.slice(offset, offset + 6);
    await Promise.all(chunk.map(async link => {
      try {
        const stats = await api(`/api/links/${link.id}/stats`);
        state.stats.set(link.id, stats);
        total += Number(stats.visits?.total || 0);
      } catch {}
      loaded += 1;
    }));
    $('#visitCount').textContent = loaded === active.length ? total.toLocaleString() : `${total.toLocaleString()}+`;
    renderLinks();
  }
  if (!active.length) $('#visitCount').textContent = '0';
}

function fillEditForm(link) {
  const form = $('#editForm');
  form.elements.id.value = link.id;
  form.elements.longUrl.value = link.long_url;
  form.elements.title.value = link.title || '';
  form.elements.tags.value = (link.tags || []).join(', ');
  form.elements.notes.value = link.notes || '';
  form.elements.expiresAt.value = toLocalInput(link.expires_at);
  form.elements.maxVisits.value = link.max_visits || '';
  $('#editTitle').textContent = link.short_url;
}

function visitsArray(payload) {
  if (Array.isArray(payload?.visits?.data)) return payload.visits.data;
  if (Array.isArray(payload?.data)) return payload.data;
  if (Array.isArray(payload?.visits)) return payload.visits;
  return [];
}

function visitLocation(visit) {
  const location = visit.visitLocation || visit.location || {};
  return [location.cityName || location.city, location.regionName || location.region, location.countryName || location.countryCode || location.country].filter(Boolean).join(', ') || 'Unknown';
}

function visitSource(visit) {
  return visit.referer || visit.referrer || visit.visitedUrl || 'Direct / unknown';
}

function drawVisitBars(visits) {
  const days = [];
  for (let i = 6; i >= 0; i -= 1) {
    const date = new Date();
    date.setHours(0, 0, 0, 0);
    date.setDate(date.getDate() - i);
    days.push({ key: date.toISOString().slice(0, 10), label: date.toLocaleDateString(undefined, { weekday: 'short' }), count: 0 });
  }
  for (const visit of visits) {
    const date = new Date(visit.date || visit.visitedAt || visit.createdAt);
    if (!Number.isFinite(date.getTime())) continue;
    const key = new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 10);
    const day = days.find(item => item.key === key);
    if (day) day.count += 1;
  }
  const max = Math.max(...days.map(day => day.count), 1);
  $('#visitBars').innerHTML = days.map(day => `<div class="visit-bar-col"><div class="visit-bar-value">${day.count}</div><div class="visit-bar-track"><div class="visit-bar-fill" style="height:${Math.max(4, Math.round(day.count / max * 100))}%"></div></div><div class="visit-bar-label">${escapeHtml(day.label)}</div></div>`).join('');
}

async function openDetails(link) {
  state.currentDetail = link;
  $('#detailTitle').textContent = link.short_url;
  $('#detailSummary').innerHTML = '<div class="empty">Loading analytics…</div>';
  $('#visitTable').innerHTML = '<div class="empty">Loading visits…</div>';
  $('#visitBars').innerHTML = '';
  $('#detailDialog').showModal();
  try {
    const [stats, visitPayload] = await Promise.all([api(`/api/links/${link.id}/stats`), api(`/api/links/${link.id}/visits?itemsPerPage=100`)]);
    state.stats.set(link.id, stats);
    const visits = visitsArray(visitPayload);
    const total = Number(stats.visits?.total || 0);
    const humans = Number(stats.visits?.nonBots || 0);
    const bots = Number(stats.visits?.bots || 0);
    $('#detailSummary').innerHTML = `<div class="detail-kpi"><span>Total visits</span><strong>${total.toLocaleString()}</strong></div><div class="detail-kpi"><span>Human</span><strong>${humans.toLocaleString()}</strong></div><div class="detail-kpi"><span>Bots</span><strong>${bots.toLocaleString()}</strong></div><div class="detail-kpi"><span>State</span><strong>${escapeHtml(statusLabel(link))}</strong></div>`;
    drawVisitBars(visits);
    $('#visitMeta').textContent = `${visits.length.toLocaleString()} recent visit${visits.length === 1 ? '' : 's'} loaded`;
    if (!visits.length) $('#visitTable').innerHTML = '<div class="empty">No visit records yet.</div>';
    else $('#visitTable').innerHTML = visits.slice(0, 100).map(visit => `<div class="visit-row"><div><strong>${escapeHtml(humanDate(visit.date || visit.visitedAt || visit.createdAt))}</strong><span>${visit.potentialBot ? 'Potential bot' : 'Human / unknown'}</span></div><div><strong>${escapeHtml(visitLocation(visit))}</strong><span>${escapeHtml(visitSource(visit))}</span></div></div>`).join('');
  } catch (error) {
    $('#detailSummary').innerHTML = `<div class="empty error-text">${escapeHtml(error.message)}</div>`;
    $('#visitTable').innerHTML = '';
  }
}

function parseBulkInput(text) {
  return String(text || '').split(/\r?\n/).map(line => line.trim()).filter(Boolean).slice(0, 100).map(line => {
    const parts = line.split(',').map(part => part.trim());
    return { longUrl: parts[0], ...(parts[1] ? { customSlug: parts[1] } : {}), ...(parts[2] ? { title: parts.slice(2).join(',') } : {}) };
  });
}

async function loadApiTokens() {
  try {
    const data = await api('/api/account/api-tokens');
    const root = $('#apiTokenList');
    if (!data.tokens.length) { root.innerHTML = '<div class="empty">No API tokens yet.</div>'; return; }
    root.innerHTML = data.tokens.map(token => `<div class="token-row"><div><strong>${escapeHtml(token.name)}</strong><div class="small muted mono">${escapeHtml(token.token_prefix)}…</div><div class="small muted">${escapeHtml((token.scopes || []).join(', '))} · created ${escapeHtml(humanDate(token.created_at))} · last used ${escapeHtml(humanDate(token.last_used_at))}</div></div><div>${token.revoked_at ? '<span class="state-chip state-disabled">Revoked</span>' : `<button class="btn btn-sm btn-danger" data-revoke-token="${escapeHtml(token.id)}">Revoke</button>`}</div></div>`).join('');
  } catch (error) { $('#apiTokenList').innerHTML = `<div class="empty error-text">${escapeHtml(error.message)}</div>`; }
}

async function loadReports() {
  try {
    const data = await api('/api/admin/reports');
    const root = $('#reportList');
    if (!data.reports.length) { root.innerHTML = '<div class="empty">No abuse reports. Good.</div>'; return; }
    root.innerHTML = data.reports.map(report => `<div class="admin-report"><div><strong>${escapeHtml(report.short_code)}</strong> <span class="pill">${escapeHtml(report.status)}</span> <span class="small muted">${escapeHtml(report.category || 'other')}</span><p>${escapeHtml(report.reason)}</p><div class="small muted">${escapeHtml(report.long_url || 'Unknown destination')} · ${escapeHtml(humanDate(report.created_at))}</div></div><div><select class="input" data-report-status="${escapeHtml(report.id)}"><option ${report.status === 'open' ? 'selected' : ''}>open</option><option ${report.status === 'reviewing' ? 'selected' : ''}>reviewing</option><option ${report.status === 'resolved' ? 'selected' : ''}>resolved</option><option ${report.status === 'dismissed' ? 'selected' : ''}>dismissed</option></select>${report.link_id ? `<button class="btn btn-sm btn-danger" data-admin-disable="${escapeHtml(report.link_id)}">Disable link</button>` : ''}</div></div>`).join('');
  } catch (error) { flash(error.message, true); }
}

async function loadUsers(q = '') {
  try {
    const data = await api(`/api/admin/users${q ? `?q=${encodeURIComponent(q)}` : ''}`);
    const root = $('#userList');
    root.innerHTML = data.users.map(user => `<div class="admin-report"><div><strong>${escapeHtml(user.name)}</strong><div class="small">${escapeHtml(user.email)} · ${escapeHtml(user.plan)} · ${user.email_verified_at ? 'verified' : 'unverified'}${user.mfa_enabled_at ? ' · MFA' : ''}${user.is_admin ? ' · admin' : ''}</div>${user.suspended_at ? `<div class="small error-text">Suspended: ${escapeHtml(user.suspension_reason || 'policy enforcement')}</div>` : ''}</div><div>${user.is_admin ? '' : user.suspended_at ? `<button class="btn btn-sm" data-unsuspend-user="${escapeHtml(user.id)}">Unsuspend</button>` : `<button class="btn btn-sm btn-danger" data-suspend-user="${escapeHtml(user.id)}">Suspend</button>`}</div></div>`).join('') || '<div class="empty">No users found.</div>';
  } catch (error) { flash(error.message, true); }
}

// Authentication events -------------------------------------------------------------------------
$('#authSwitch').addEventListener('click', () => setAuthMode($('#signupForm').classList.contains('hidden') ? 'signup' : 'login'));
$('#loginForm').addEventListener('submit', async event => {
  event.preventDefault();
  try {
    const data = withTurnstile(formJson(event.currentTarget), 'login');
    const result = await api('/api/auth/login', { method: 'POST', body: JSON.stringify(data) });
    if (result.mfaRequired) {
      $('#loginForm').classList.add('hidden'); $('#signupForm').classList.add('hidden'); $('#mfaLoginForm').classList.remove('hidden');
      $('#mfaLoginForm').elements.challengeToken.value = result.challengeToken;
      $('#authTitle').textContent = 'One more step.'; $('#authCopy').textContent = 'Enter your authenticator or a recovery code.';
      return;
    }
    location.href = '/app';
  } catch (error) { authError(error.message); resetTurnstile('login'); }
});
$('#mfaLoginForm').addEventListener('submit', async event => {
  event.preventDefault();
  try { await api('/api/auth/mfa', { method: 'POST', body: JSON.stringify(formJson(event.currentTarget)) }); location.href = '/app'; }
  catch (error) { authError(error.message); }
});
$('#mfaBackBtn').addEventListener('click', () => setAuthMode('login'));
$('#signupForm').addEventListener('submit', async event => {
  event.preventDefault();
  try {
    const data = withTurnstile(formJson(event.currentTarget), 'signup');
    const result = await api('/api/auth/register', { method: 'POST', body: JSON.stringify(data) });
    state.user = result.user;
    $('#authView').classList.add('hidden'); $('#dashboard').classList.remove('hidden');
    await refreshMe(); showSection('linksSection'); await loadLinks();
    if (result.verificationRequired) flash(result.verificationSent ? 'Check your inbox to verify your email.' : 'Account created, but the verification email could not be sent. Use Resend verification.', !result.verificationSent);
  } catch (error) { authError(error.message); resetTurnstile('signup'); }
});
$('#logoutBtn').addEventListener('click', async () => { await api('/api/auth/logout', { method: 'POST' }); location.href = '/app#login'; });

// Navigation and link workspace -----------------------------------------------------------------
$$('.side-link[data-section]').forEach(button => button.addEventListener('click', () => showSection(button.dataset.section)));
$('#globalCreateBtn').addEventListener('click', () => { showSection('linksSection'); $('#createPanel').scrollIntoView({ behavior: 'smooth' }); $('#createLinkForm').elements.longUrl.focus(); });
$('#refreshBtn').addEventListener('click', loadLinks);
$('#linkFilters').addEventListener('submit', event => { event.preventDefault(); const data = formJson(event.currentTarget); state.q = String(data.q || '').trim(); state.status = data.status || 'all'; state.tag = String(data.tag || '').trim(); state.offset = 0; loadLinks(); });
$('#clearFiltersBtn').addEventListener('click', () => { $('#linkFilters').reset(); state.q = ''; state.status = 'all'; state.tag = ''; state.offset = 0; loadLinks(); });
$('#prevPageBtn').addEventListener('click', () => { state.offset = Math.max(0, state.offset - state.limit); loadLinks(); });
$('#nextPageBtn').addEventListener('click', () => { state.offset += state.limit; loadLinks(); });

$('#createLinkForm').addEventListener('submit', async event => {
  event.preventDefault();
  const button = event.currentTarget.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const data = formJson(event.currentTarget);
    if (data.expiresAt && !isPro()) throw new Error('Link expiration is a Pro feature.');
    if (data.maxVisits && !isPro()) throw new Error('Max-visit limits are a Pro feature.');
    await api('/api/links', { method: 'POST', body: JSON.stringify(data) });
    event.currentTarget.reset(); flash('Short link created.'); state.offset = 0; await loadLinks();
  } catch (error) { flash(error.message, true); }
  finally { button.disabled = !userEligible(); }
});

$('#linkList').addEventListener('click', async event => {
  const button = event.target.closest('[data-action]');
  if (!button) return;
  const row = button.closest('[data-id]');
  const link = state.links.find(item => item.id === row?.dataset.id);
  if (!link) return;
  const action = button.dataset.action;
  try {
    if (action === 'copy') { await navigator.clipboard.writeText(link.short_url); flash('Copied short URL.'); }
    if (action === 'qr') { $('#qrBox').innerHTML = `<img src="/api/links/${encodeURIComponent(link.id)}/qr.svg" alt="QR code">`; $('#qrUrl').textContent = link.short_url; $('#qrDialog').showModal(); }
    if (action === 'details') await openDetails(link);
    if (action === 'edit') { fillEditForm(link); $('#editDialog').showModal(); }
    if (action === 'archive') { await api(`/api/links/${link.id}/archive`, { method: 'POST' }); flash('Link archived. It still redirects.'); await loadLinks(); }
    if (action === 'unarchive') { await api(`/api/links/${link.id}/unarchive`, { method: 'POST' }); flash('Link returned to the main library.'); await loadLinks(); }
    if (action === 'disable' && confirm(`Disable ${link.short_url}? Existing copies will stop redirecting.`)) { await api(`/api/links/${link.id}`, { method: 'DELETE' }); flash('Link disabled.'); await loadLinks(); }
    if (action === 'restore' && confirm(`Restore ${link.short_url}?`)) { await api(`/api/links/${link.id}/restore`, { method: 'POST' }); flash('Link restored.'); await loadLinks(); }
  } catch (error) { flash(error.message, true); }
});

$('#editForm').addEventListener('submit', async event => {
  event.preventDefault();
  const data = formJson(event.currentTarget);
  if ((data.expiresAt || data.maxVisits) && !isPro()) return flash('Expiry and max-visit controls require Pro.', true);
  try {
    await api(`/api/links/${data.id}`, { method: 'PATCH', body: JSON.stringify({ longUrl: data.longUrl, title: data.title, tags: data.tags, notes: data.notes, expiresAt: data.expiresAt, maxVisits: data.maxVisits }) });
    $('#editDialog').close(); flash('Link updated.'); await loadLinks();
  } catch (error) { flash(error.message, true); }
});

$('#bulkBtn').addEventListener('click', () => {
  if (!isPro()) { showSection('billingPanel'); return flash('Bulk creation is a Pro feature.', true); }
  $('#bulkResults').innerHTML = ''; $('#bulkStatus').textContent = ''; $('#bulkDialog').showModal();
});
$('#bulkSubmitBtn').addEventListener('click', async () => {
  const links = parseBulkInput($('#bulkInput').value);
  if (!links.length) return flash('Paste at least one destination.', true);
  $('#bulkSubmitBtn').disabled = true; $('#bulkStatus').textContent = `Creating ${links.length}…`;
  try {
    const result = await api('/api/links/bulk', { method: 'POST', body: JSON.stringify({ links }) });
    $('#bulkStatus').textContent = `${result.created} created · ${result.failed} failed`;
    $('#bulkResults').innerHTML = result.results.map((item, index) => item.ok ? `<div class="bulk-result ok">✓ ${escapeHtml(item.link.short_url)}</div>` : `<div class="bulk-result fail">✕ Row ${index + 1}: ${escapeHtml(item.message)}</div>`).join('');
    await loadLinks();
  } catch (error) { $('#bulkStatus').textContent = error.message; }
  finally { $('#bulkSubmitBtn').disabled = false; }
});

// Developer --------------------------------------------------------------------------------------
$('#refreshTokensBtn').addEventListener('click', loadApiTokens);
$('#apiTokenForm').addEventListener('submit', async event => {
  event.preventDefault();
  if (!isPro()) return flash('Developer API tokens require QH8Z Pro.', true);
  const data = formJson(event.currentTarget);
  const scopes = [];
  if (event.currentTarget.elements.read.checked) scopes.push('links:read');
  if (event.currentTarget.elements.write.checked) scopes.push('links:write');
  try {
    const result = await api('/api/account/api-tokens', { method: 'POST', body: JSON.stringify({ name: data.name, expiresInDays: data.expiresInDays, scopes }) });
    $('#newApiToken').textContent = result.token; $('#apiTokenDialog').showModal(); event.currentTarget.reset(); event.currentTarget.elements.read.checked = true; event.currentTarget.elements.write.checked = true; await loadApiTokens();
  } catch (error) { flash(error.message, true); }
});
$('#copyApiTokenBtn').addEventListener('click', async () => { await navigator.clipboard.writeText($('#newApiToken').textContent); flash('API token copied.'); });
$('#apiTokenList').addEventListener('click', async event => {
  const button = event.target.closest('[data-revoke-token]');
  if (!button || !confirm('Revoke this API token? Applications using it will immediately lose access.')) return;
  try { await api(`/api/account/api-tokens/${button.dataset.revokeToken}`, { method: 'DELETE' }); flash('API token revoked.'); await loadApiTokens(); }
  catch (error) { flash(error.message, true); }
});

// Billing ----------------------------------------------------------------------------------------
$('#upgradeBtn').addEventListener('click', async () => { try { const data = await api('/api/billing/checkout', { method: 'POST' }); location.href = data.url; } catch (error) { flash(error.message, true); } });
$('#portalBtn').addEventListener('click', async () => { try { const data = await api('/api/billing/portal', { method: 'POST' }); location.href = data.url; } catch (error) { flash(error.message, true); } });

// Account ----------------------------------------------------------------------------------------
$('#resendVerificationBtn').addEventListener('click', async () => { try { await api('/api/auth/resend-verification', { method: 'POST' }); flash('Verification email sent.'); } catch (error) { flash(error.message, true); } });
$('#acceptTermsBtn').addEventListener('click', async () => { if (!confirm('I have reviewed and accept the current QH8Z Terms.')) return; try { const data = await api('/api/account/accept-terms', { method: 'POST', body: JSON.stringify({ acceptTerms: true }) }); state.user = data.user; updateEligibilityUi(); flash('Current Terms accepted.'); } catch (error) { flash(error.message, true); } });
$('#mfaSetupBtn').addEventListener('click', () => { $('#mfaStartForm').classList.remove('hidden'); $('#mfaConfirmView').classList.add('hidden'); $('#mfaRecoveryView').classList.add('hidden'); $('#mfaStartForm').reset(); $('#mfaConfirmForm').reset(); $('#mfaSetupDialog').showModal(); });
$('#mfaStartForm').addEventListener('submit', async event => { event.preventDefault(); try { const result = await api('/api/account/mfa/setup', { method: 'POST', body: JSON.stringify(formJson(event.currentTarget)) }); $('#mfaQr').src = result.qrDataUrl; $('#mfaSecret').textContent = result.secret; $('#mfaStartForm').classList.add('hidden'); $('#mfaConfirmView').classList.remove('hidden'); } catch (error) { flash(error.message, true); } });
$('#mfaConfirmForm').addEventListener('submit', async event => { event.preventDefault(); try { const result = await api('/api/account/mfa/confirm', { method: 'POST', body: JSON.stringify(formJson(event.currentTarget)) }); $('#mfaConfirmView').classList.add('hidden'); $('#mfaRecoveryCodes').textContent = result.recoveryCodes.join('\n'); $('#mfaRecoveryView').classList.remove('hidden'); await refreshMe(); flash('Two-factor authentication enabled.'); } catch (error) { flash(error.message, true); } });
$('#mfaDisableBtn').addEventListener('click', async () => { const password = prompt('Enter your current password:'); if (!password) return; const code = prompt('Enter an authenticator or recovery code:'); if (!code) return; if (!confirm('Disable two-factor authentication?')) return; try { await api('/api/account/mfa/disable', { method: 'POST', body: JSON.stringify({ password, code }) }); await refreshMe(); flash('Two-factor authentication disabled.'); } catch (error) { flash(error.message, true); } });
$('#passwordForm').addEventListener('submit', async event => { event.preventDefault(); try { const data = formJson(event.currentTarget); if (state.user?.mfaEnabled) { data.mfaCode = prompt('Enter an authenticator or recovery code to change your password:') || ''; if (!data.mfaCode) return; } await api('/api/account/password', { method: 'POST', body: JSON.stringify(data) }); event.currentTarget.reset(); flash('Password changed. Other sessions were signed out.'); } catch (error) { flash(error.message, true); } });
$('#deleteAccountBtn').addEventListener('click', async () => { const password = prompt('Enter your password to permanently delete your QH8Z account:'); if (!password) return; let mfaCode = ''; if (state.user.mfaEnabled) { mfaCode = prompt('Enter an authenticator or recovery code:') || ''; if (!mfaCode) return; } if (!confirm('This cannot be undone. Delete the account, disable active links, and cancel billing?')) return; try { await api('/api/account', { method: 'DELETE', body: JSON.stringify({ password, mfaCode }) }); location.href = '/'; } catch (error) { flash(error.message, true); } });

// Admin ------------------------------------------------------------------------------------------
$('#adminRefreshBtn').addEventListener('click', () => { loadReports(); loadUsers(); });
$('#adminUserSearch').addEventListener('submit', event => { event.preventDefault(); loadUsers(formJson(event.currentTarget).q || ''); });
$('#reportList').addEventListener('change', async event => { const element = event.target.closest('[data-report-status]'); if (!element) return; try { await api(`/api/admin/reports/${element.dataset.reportStatus}`, { method: 'PATCH', body: JSON.stringify({ status: element.value }) }); flash('Report updated.'); } catch (error) { flash(error.message, true); } });
$('#reportList').addEventListener('click', async event => { const element = event.target.closest('[data-admin-disable]'); if (!element || !confirm('Disable this reported short link?')) return; try { await api(`/api/admin/links/${element.dataset.adminDisable}/disable`, { method: 'POST' }); flash('Reported link disabled.'); await loadReports(); } catch (error) { flash(error.message, true); } });
$('#userList').addEventListener('click', async event => {
  const suspend = event.target.closest('[data-suspend-user]'); const unsuspend = event.target.closest('[data-unsuspend-user]');
  try {
    if (suspend) { const reason = prompt('Suspension reason:', 'Abuse or policy enforcement'); if (!reason) return; await api(`/api/admin/users/${suspend.dataset.suspendUser}/suspend`, { method: 'POST', body: JSON.stringify({ reason }) }); flash('User suspended and active links disabled.'); await loadUsers(); }
    if (unsuspend) { await api(`/api/admin/users/${unsuspend.dataset.unsuspendUser}/unsuspend`, { method: 'POST' }); flash('User unsuspended. Previously disabled links remain disabled.'); await loadUsers(); }
  } catch (error) { flash(error.message, true); }
});

// Dialogs ----------------------------------------------------------------------------------------
document.addEventListener('click', event => { const button = event.target.closest('[data-close-dialog]'); if (button) document.getElementById(button.dataset.closeDialog)?.close(); });

boot().catch(error => authError(error.message));

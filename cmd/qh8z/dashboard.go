package main

import (
	"net/http"
)

func (a *app) dashboard(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authorize(r, "", true); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>qh8z dashboard</title>
<style>
:root{color-scheme:dark;font-family:Inter,ui-sans-serif,system-ui,sans-serif;background:#090b0e;color:#f7f8fa}*{box-sizing:border-box}body{margin:0;background:linear-gradient(180deg,#0b0e12,#07090c 60%);min-height:100vh}button,input,select{font:inherit}.shell{max-width:1180px;margin:auto;padding:28px 22px 64px}.top{display:flex;align-items:center;justify-content:space-between;gap:18px;margin-bottom:28px}.brand{display:flex;gap:14px;align-items:center}.logo{font-size:34px;font-weight:900;letter-spacing:-.06em}.muted{color:#9099a8}.pill{border:1px solid #2c3440;border-radius:999px;padding:6px 10px;font-size:12px;background:#12171e}.grid{display:grid;gap:16px}.metrics{grid-template-columns:repeat(4,minmax(0,1fr));margin-bottom:16px}.card{border:1px solid #202731;background:#10141a;border-radius:16px;padding:18px;box-shadow:0 15px 40px rgba(0,0,0,.2)}.metric strong{display:block;font-size:30px;margin-top:8px}.layout{grid-template-columns:minmax(0,1.6fr) minmax(280px,.8fr)}.stack{display:grid;gap:16px}.section-title{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:14px}.section-title h2{margin:0;font-size:18px}.form-row{display:flex;gap:10px;flex-wrap:wrap}.input{background:#0a0d11;color:#f7f8fa;border:1px solid #2a323d;border-radius:10px;padding:10px 12px;min-width:0}.grow{flex:1}.btn{border:0;border-radius:10px;padding:10px 13px;font-weight:750;cursor:pointer;background:#f4f6f8;color:#101216}.btn.secondary{background:#202731;color:#f4f6f8}.btn.danger{background:#36191d;color:#ffd7db}.btn:disabled{opacity:.45;cursor:not-allowed}.table-wrap{overflow:auto}.table{width:100%;border-collapse:collapse;font-size:13px}.table th,.table td{text-align:left;border-top:1px solid #202731;padding:11px 8px;vertical-align:top}.table th{color:#8e98a8;font-weight:650}.url{max-width:360px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.actions{display:flex;gap:7px;flex-wrap:wrap}.mini{padding:6px 8px;font-size:12px}.bars{display:flex;gap:5px;align-items:end;height:120px;padding-top:12px}.bar{flex:1;min-width:4px;background:#d7dce2;border-radius:4px 4px 1px 1px;opacity:.9}.list{display:grid;gap:10px}.list-row{display:flex;justify-content:space-between;gap:12px;border-top:1px solid #202731;padding-top:10px}.progress{height:8px;background:#252d37;border-radius:99px;overflow:hidden;margin:10px 0}.progress span{display:block;height:100%;background:#f1f4f7}.notice{display:none;margin-bottom:16px;padding:12px 14px;border-radius:12px;border:1px solid #385071;background:#101b2b}.notice.show{display:block}.empty{padding:22px 4px;color:#8993a2;text-align:center}.domain-verify{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:11px;overflow-wrap:anywhere;color:#cbd2dc}@media(max-width:850px){.metrics{grid-template-columns:repeat(2,1fr)}.layout{grid-template-columns:1fr}}@media(max-width:520px){.metrics{grid-template-columns:1fr 1fr}.top{align-items:flex-start}.form-row{display:grid}.input,.btn{width:100%}}
</style>
<script src="/assets/dashboard.js" defer></script>
</head>
<body>
<main class="shell">
  <header class="top"><div class="brand"><div class="logo">qh8z</div><div><div>Link workspace</div><div class="muted" id="workspace-name">Loading…</div></div></div><div><span class="pill" id="plan-pill">Free</span></div></header>
  <div class="notice" id="notice"></div>
  <section class="grid metrics">
    <div class="card metric"><span class="muted">Links</span><strong id="m-links">—</strong><span class="muted" id="m-links-sub"></span></div>
    <div class="card metric"><span class="muted">Visits</span><strong id="m-visits">—</strong><span class="muted">selected period</span></div>
    <div class="card metric"><span class="muted">Active</span><strong id="m-active">—</strong><span class="muted">redirecting links</span></div>
    <div class="card metric"><span class="muted">Domains</span><strong id="m-domains">—</strong><span class="muted" id="m-domains-sub"></span></div>
  </section>
  <div class="grid layout">
    <div class="stack">
      <section class="card">
        <div class="section-title"><h2>Create a short link</h2><span class="muted" id="create-plan-note"></span></div>
        <form class="form-row" id="create-form"><input class="input grow" id="create-url" type="url" required placeholder="https://example.com/long-path"><input class="input" id="create-slug" placeholder="custom-slug (optional)"><select class="input" id="create-domain"><option value="">qh8z primary domain</option></select><button class="btn">Shorten</button></form>
      </section>
      <section class="card">
        <div class="section-title"><h2>Links</h2><button class="btn secondary mini" id="refresh">Refresh</button></div>
        <div class="table-wrap"><table class="table"><thead><tr><th>Short link</th><th>Destination</th><th>Visits</th><th>Status</th><th>Actions</th></tr></thead><tbody id="links-body"></tbody></table></div>
      </section>
      <section class="card">
        <div class="section-title"><h2>Visits</h2><select class="input" id="analytics-days"><option value="7">7 days</option><option value="30">30 days</option><option value="90">90 days</option></select></div>
        <div class="bars" id="daily-bars"></div>
      </section>
    </div>
    <aside class="stack">
      <section class="card">
        <div class="section-title"><h2>Plan & usage</h2><span class="pill" id="billing-status">active</span></div>
        <div><span class="muted">Links</span><div class="progress"><span id="link-progress" style="width:0"></span></div><div class="muted" id="link-usage"></div></div>
        <div style="margin-top:16px"><span class="muted">Custom domains</span><div class="progress"><span id="domain-progress" style="width:0"></span></div><div class="muted" id="domain-usage"></div></div>
        <div class="form-row" style="margin-top:18px"><button class="btn" id="upgrade">Upgrade to Pro</button><button class="btn secondary" id="manage-billing">Manage billing</button></div>
      </section>
      <section class="card">
        <div class="section-title"><h2>Top links</h2></div><div class="list" id="top-links"></div>
      </section>
      <section class="card">
        <div class="section-title"><h2>Referrers</h2></div><div class="list" id="referrers"></div>
      </section>
      <section class="card">
        <div class="section-title"><h2>Custom domains</h2></div>
        <form class="form-row" id="domain-form"><input class="input grow" id="domain-host" placeholder="go.example.com"><button class="btn secondary">Add</button></form>
        <div class="list" id="domains" style="margin-top:14px"></div>
      </section>
    </aside>
  </div>
</main>
</body>
</html>`))
}

func (a *app) dashboardJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(`(() => {
'use strict';
const $ = (id) => document.getElementById(id);
const state = { me:null, usage:null, limits:null, analytics:null, links:[], domains:[], billing:null };
const notice = (message, error=false) => { const el=$('notice'); el.textContent=message; el.classList.add('show'); el.style.borderColor=error?'#7b3139':'#385071'; window.setTimeout(()=>el.classList.remove('show'),5000); };
const api = async (path, options={}) => { const response=await fetch(path,{credentials:'same-origin',...options,headers:{'content-type':'application/json',...(options.headers||{})}}); if(response.status===204)return null; const type=response.headers.get('content-type')||''; const data=type.includes('json')?await response.json():await response.text(); if(!response.ok)throw new Error((data&&data.error)||('Request failed: '+response.status)); return data; };
const pct = (value, limit) => !limit ? 0 : Math.min(100, Math.round((value/limit)*100));
const shortURL = (link) => link.shortUrl;

async function load() {
  try {
    const [me,usage,analytics,links,domains,billing] = await Promise.all([
      api('/api/v1/me'), api('/api/v1/usage'), api('/api/v1/analytics'), api('/api/v1/links?limit=100'), api('/api/v1/domains'), api('/api/v1/billing')
    ]);
    state.me=me; state.usage=usage.usage; state.limits=usage.limits; state.analytics=analytics.analytics; state.links=links.links||[]; state.domains=domains.domains||[]; state.billing=billing.billing;
    render();
  } catch (error) { notice(error.message,true); }
}

function render() {
  const workspace=(state.me.workspaces||[]).find(w=>w.id===state.me.auth.workspaceId)||(state.me.workspaces||[])[0];
  $('workspace-name').textContent=workspace?workspace.name:state.me.auth.workspaceId;
  $('plan-pill').textContent=(state.usage.plan||'free').toUpperCase();
  $('billing-status').textContent=state.billing.status||'active';
  $('m-links').textContent=state.usage.links;
  $('m-links-sub').textContent=state.limits.linkLimit+' plan limit';
  $('m-visits').textContent=state.analytics.periodVisits;
  $('m-active').textContent=state.analytics.activeLinks;
  $('m-domains').textContent=state.usage.customDomains;
  $('m-domains-sub').textContent=state.limits.customDomainLimit+' plan limit';
  $('link-progress').style.width=pct(state.usage.links,state.limits.linkLimit)+'%';
  $('link-usage').textContent=state.usage.links+' of '+state.limits.linkLimit;
  $('domain-progress').style.width=pct(state.usage.customDomains,state.limits.customDomainLimit)+'%';
  $('domain-usage').textContent=state.usage.customDomains+' of '+state.limits.customDomainLimit;
  $('create-plan-note').textContent=state.usage.links+' / '+state.limits.linkLimit+' links used';
  $('upgrade').disabled=state.usage.plan==='pro';
  $('manage-billing').disabled=!state.billing.currentPeriodEnd && state.usage.plan!=='pro';
  const days=$('analytics-days'); Array.from(days.options).forEach(o=>o.disabled=Number(o.value)>state.limits.analyticsDays); if(Number(days.value)>state.limits.analyticsDays)days.value=String(state.limits.analyticsDays);
  renderLinks(); renderDomains(); renderAnalytics(); renderDomainSelect();
}

function renderLinks() {
  const body=$('links-body'); body.replaceChildren();
  if(!state.links.length){const tr=document.createElement('tr'),td=document.createElement('td');td.colSpan=5;td.className='empty';td.textContent='No links yet.';tr.append(td);body.append(tr);return;}
  for(const link of state.links){
    const tr=document.createElement('tr');
    const short=document.createElement('td'); const a=document.createElement('a');a.href=shortURL(link);a.textContent=shortURL(link);a.target='_blank';a.rel='noopener';short.append(a);
    const dest=document.createElement('td');dest.className='url';dest.title=link.url;dest.textContent=link.url;
    const visits=document.createElement('td');visits.textContent=String(link.visits);
    const status=document.createElement('td');status.textContent=link.suspendedAt?'Suspended':link.disabledAt?'Disabled':'Active';
    const actions=document.createElement('td');actions.className='actions';
    actions.append(actionButton('Copy',()=>navigator.clipboard.writeText(shortURL(link)).then(()=>notice('Short link copied.'))));
    const qr=actionButton('QR',()=>window.open('/api/v1/links/'+encodeURIComponent(link.slug)+'/qr.png','_blank','noopener'));actions.append(qr);
    actions.append(actionButton(link.disabledAt?'Enable':'Disable',()=>toggleLink(link)));
    actions.append(actionButton('Edit',()=>editLink(link)));
    const del=actionButton('Delete',()=>deleteLink(link));del.classList.add('danger');actions.append(del);
    tr.append(short,dest,visits,status,actions); body.append(tr);
  }
}
function actionButton(label, handler){const b=document.createElement('button');b.className='btn secondary mini';b.type='button';b.textContent=label;b.addEventListener('click',handler);return b;}
async function toggleLink(link){try{await api('/api/v1/links/'+encodeURIComponent(link.slug),{method:'PATCH',body:JSON.stringify({disabled:!link.disabledAt})});await load();}catch(e){notice(e.message,true);}}
async function editLink(link){const next=window.prompt('Destination URL',link.url);if(!next||next===link.url)return;try{await api('/api/v1/links/'+encodeURIComponent(link.slug),{method:'PATCH',body:JSON.stringify({url:next})});await load();notice('Link updated.');}catch(e){notice(e.message,true);}}
async function deleteLink(link){if(!window.confirm('Delete '+link.shortUrl+'? This cannot be undone.'))return;try{await api('/api/v1/links/'+encodeURIComponent(link.slug),{method:'DELETE'});await load();notice('Link deleted.');}catch(e){notice(e.message,true);}}

function renderDomains(){const root=$('domains');root.replaceChildren();if(!state.domains.length){const e=document.createElement('div');e.className='empty';e.textContent=state.limits.customDomainLimit?'No custom domains.':'Custom domains are included with Pro.';root.append(e);return;}for(const domain of state.domains){const row=document.createElement('div');row.className='list-row';const info=document.createElement('div');const host=document.createElement('strong');host.textContent=domain.host;info.append(host);const sub=document.createElement('div');sub.className='muted';sub.textContent=domain.verifiedAt?'Verified':'Pending DNS verification';info.append(sub);if(domain.verification){const txt=document.createElement('div');txt.className='domain-verify';txt.textContent=domain.verification.name+' = '+domain.verification.value;info.append(txt);}const acts=document.createElement('div');acts.className='actions';if(!domain.verifiedAt)acts.append(actionButton('Verify',()=>verifyDomain(domain)));const del=actionButton('Remove',()=>deleteDomain(domain));del.classList.add('danger');acts.append(del);row.append(info,acts);root.append(row);}}
function renderDomainSelect(){const select=$('create-domain');const selected=select.value;select.replaceChildren();const primary=document.createElement('option');primary.value='';primary.textContent='qh8z primary domain';select.append(primary);for(const domain of state.domains.filter(d=>d.verifiedAt)){const option=document.createElement('option');option.value=domain.id;option.textContent=domain.host;select.append(option);}select.value=selected;}
async function verifyDomain(domain){try{const result=await api('/api/v1/domains/'+encodeURIComponent(domain.id)+'/verify',{method:'POST',body:'{}'});if(!result.verified)notice('TXT record not found yet.',true);await load();}catch(e){notice(e.message,true);}}
async function deleteDomain(domain){if(!window.confirm('Remove '+domain.host+'? Existing branded links will fall back to the primary domain.'))return;try{await api('/api/v1/domains/'+encodeURIComponent(domain.id),{method:'DELETE'});await load();}catch(e){notice(e.message,true);}}

function renderAnalytics(){const max=Math.max(1,...(state.analytics.daily||[]).map(d=>d.visits));const bars=$('daily-bars');bars.replaceChildren();if(!state.analytics.daily.length){const empty=document.createElement('div');empty.className='empty';empty.textContent='No visits in this period.';bars.append(empty);}for(const day of state.analytics.daily){const bar=document.createElement('div');bar.className='bar';bar.style.height=Math.max(4,Math.round((day.visits/max)*100))+'%';bar.title=day.date+': '+day.visits+' visits';bars.append(bar);}renderList('top-links',state.analytics.topLinks||[],x=>x.slug,x=>x.visits+' visits');renderList('referrers',state.analytics.referrers||[],x=>x.referrer,x=>x.visits+' visits');}
function renderList(id,items,left,right){const root=$(id);root.replaceChildren();if(!items.length){const e=document.createElement('div');e.className='empty';e.textContent='No data yet.';root.append(e);return;}for(const item of items){const row=document.createElement('div');row.className='list-row';const l=document.createElement('span');l.textContent=left(item);const r=document.createElement('strong');r.textContent=right(item);row.append(l,r);root.append(row);}}

$('create-form').addEventListener('submit',async(e)=>{e.preventDefault();const payload={url:$('create-url').value.trim()};const slug=$('create-slug').value.trim();const domainId=$('create-domain').value;if(slug)payload.customSlug=slug;if(domainId)payload.domainId=domainId;try{const result=await api('/api/v1/links',{method:'POST',body:JSON.stringify(payload)});$('create-url').value='';$('create-slug').value='';await load();notice('Created '+result.shortUrl);}catch(err){notice(err.message,true);}});
$('domain-form').addEventListener('submit',async(e)=>{e.preventDefault();const host=$('domain-host').value.trim();if(!host)return;try{await api('/api/v1/domains',{method:'POST',body:JSON.stringify({host})});$('domain-host').value='';await load();notice('Domain added. Publish the shown TXT record, then verify.');}catch(err){notice(err.message,true);}});
$('analytics-days').addEventListener('change',async()=>{try{const data=await api('/api/v1/analytics?days='+encodeURIComponent($('analytics-days').value));state.analytics=data.analytics;renderAnalytics();$('m-visits').textContent=state.analytics.periodVisits;}catch(e){notice(e.message,true);}});
$('refresh').addEventListener('click',load);
$('upgrade').addEventListener('click',async()=>{try{const x=await api('/api/v1/billing/checkout',{method:'POST',body:'{}'});window.location.assign(x.checkout.url);}catch(e){notice(e.message,true);}});
$('manage-billing').addEventListener('click',async()=>{try{const x=await api('/api/v1/billing/portal',{method:'POST',body:'{}'});window.location.assign(x.portal.url);}catch(e){notice(e.message,true);}});
load();
})();`))
}

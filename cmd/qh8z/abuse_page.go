package main

import "net/http"

func (a *app) abuseReportPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Report abuse · qh8z</title><style>
:root{color-scheme:dark}*{box-sizing:border-box}body{margin:0;background:#090b10;color:#e8edf5;font:16px/1.55 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}a{color:#9bbcff}main{width:min(680px,calc(100% - 32px));margin:0 auto;padding:48px 0 64px}.brand{font-size:24px;font-weight:850;color:#fff;text-decoration:none}section{margin-top:28px;background:#10141c;border:1px solid #252b36;border-radius:18px;padding:clamp(22px,5vw,42px)}h1{font-size:clamp(34px,7vw,50px);line-height:1.05;margin:0 0 16px}.muted{color:#9ca8ba}label{display:block;font-weight:700;margin:20px 0 7px}input,select,textarea,button{width:100%;font:inherit;border-radius:10px}input,select,textarea{border:1px solid #30394a;background:#0b0f16;color:#fff;padding:11px 12px}textarea{min-height:150px;resize:vertical}button{margin-top:22px;border:0;background:#f2f5f8;color:#101216;padding:12px 14px;font-weight:800;cursor:pointer}button:disabled{opacity:.55;cursor:wait}.status{display:none;margin-top:20px;border-radius:10px;padding:12px 14px}.status.show{display:block}.ok{background:#10241a;border:1px solid #285d3c}.error{background:#2a1418;border:1px solid #74313b}footer{margin-top:28px;color:#8691a3;font-size:14px;display:flex;gap:12px;flex-wrap:wrap}
</style><script src="/assets/abuse.js" defer></script></head><body><main><a class="brand" href="/">qh8z</a><section><h1>Report an abusive link</h1><p class="muted">Use this form for phishing, malware, scams, spam, or other abusive qh8z links. For urgent issues you may also email <a href="mailto:abuse@qh8z.com">abuse@qh8z.com</a>.</p><form id="abuse-form"><label for="link">qh8z short link or slug</label><input id="link" name="link" required autocomplete="off" placeholder="https://qh8z.com/example or example"><label for="category">Category</label><select id="category" name="category" required><option value="phishing">Phishing</option><option value="malware">Malware</option><option value="scam">Scam or fraud</option><option value="spam">Spam</option><option value="other">Other</option></select><label for="details">Details</label><textarea id="details" name="details" maxlength="2000" placeholder="Describe what happened and include only information needed to investigate."></textarea><label for="email">Your email (optional)</label><input id="email" name="email" type="email" autocomplete="email" placeholder="you@example.com"><button id="submit" type="submit">Submit report</button></form><div id="status" class="status" role="status" aria-live="polite"></div></section><footer><a href="/acceptable-use">Acceptable Use</a><a href="/privacy">Privacy</a><a href="/terms">Terms</a></footer></main></body></html>`))
}

func (a *app) abuseReportJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(`(() => {
'use strict';
const form=document.getElementById('abuse-form');
const status=document.getElementById('status');
const submit=document.getElementById('submit');
function slugFrom(value){
  const raw=value.trim();
  if(!raw)return '';
  try{
    const url=new URL(raw);
    const parts=url.pathname.split('/').filter(Boolean);
    return parts.length?parts[parts.length-1]:'';
  }catch(_){
    return raw.replace(/^\/+|\/+$/g,'');
  }
}
form.addEventListener('submit',async(event)=>{
  event.preventDefault();
  status.className='status';
  status.textContent='';
  submit.disabled=true;
  try{
    const response=await fetch('/api/v1/abuse-reports',{
      method:'POST',credentials:'same-origin',headers:{'content-type':'application/json'},
      body:JSON.stringify({
        slug:slugFrom(document.getElementById('link').value),
        category:document.getElementById('category').value,
        details:document.getElementById('details').value,
        reporterEmail:document.getElementById('email').value
      })
    });
    let data={};
    try{data=await response.json();}catch(_){}
    if(!response.ok)throw new Error(data.error||('Request failed: '+response.status));
    form.reset();
    status.textContent='Report accepted. Thank you. qh8z will review it under the abuse-response process.';
    status.className='status show ok';
  }catch(error){
    status.textContent=error.message||'Unable to submit the report.';
    status.className='status show error';
  }finally{submit.disabled=false;}
});
})();`))
}

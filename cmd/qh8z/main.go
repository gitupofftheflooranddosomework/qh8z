package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type link struct {
	Slug string `json:"slug"`
	URL string `json:"url"`
	ShortURL string `json:"shortUrl"`
	CreatedAt time.Time `json:"createdAt"`
	Visits int64 `json:"visits"`
}

type app struct {
	mu sync.RWMutex
	links map[string]link
	baseURL string
}

var slugPattern = regexp.MustCompile(`^[a-z0-9_-]{3,64}$`)
var reserved = map[string]bool{"api": true, "healthz": true, "admin": true, "login": true, "signup": true, "pricing": true}
const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func main() {
	port := envOr("PORT", "8080")
	a := &app{links: map[string]link{}, baseURL: strings.TrimRight(envOr("QH8Z_BASE_URL", "http://localhost:"+port), "/")}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/links", a.create)
	mux.HandleFunc("GET /api/v1/links/{slug}", a.get)
	mux.HandleFunc("GET /{slug}", a.redirect)
	mux.HandleFunc("GET /", a.home)
	log.Printf("qh8z listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, securityHeaders(mux)))
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status":"ok"}) }

func (a *app) create(w http.ResponseWriter, r *http.Request) {
	var req struct { URL string `json:"url"`; CustomSlug string `json:"customSlug"` }
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)); dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil { writeError(w, 400, "invalid JSON body"); return }
	target, err := normalizeURL(req.URL); if err != nil { writeError(w, 400, err.Error()); return }
	slug := strings.TrimSpace(req.CustomSlug)
	if slug != "" {
		if !slugPattern.MatchString(slug) || reserved[slug] { writeError(w, 400, "custom slug must be 3-64 lowercase letters, numbers, hyphens, or underscores and cannot be reserved"); return }
	} else {
		slug, err = randomSlug(7); if err != nil { writeError(w, 500, "could not generate slug"); return }
	}
	a.mu.Lock(); defer a.mu.Unlock()
	if _, exists := a.links[slug]; exists { writeError(w, 409, "slug already exists"); return }
	item := link{Slug: slug, URL: target, ShortURL: a.baseURL+"/"+slug, CreatedAt: time.Now().UTC()}
	a.links[slug] = item
	writeJSON(w, 201, item)
}

func (a *app) get(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock(); item, ok := a.links[r.PathValue("slug")]; a.mu.RUnlock()
	if !ok { writeError(w, 404, "link not found"); return }
	writeJSON(w, 200, item)
}

func (a *app) redirect(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	a.mu.Lock(); item, ok := a.links[slug]; if ok { item.Visits++; a.links[slug] = item }; a.mu.Unlock()
	if !ok { http.NotFound(w, r); return }
	http.Redirect(w, r, item.URL, http.StatusFound)
}

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" { http.NotFound(w, r); return }
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>qh8z</title><style>body{font-family:system-ui,sans-serif;background:#0b0d10;color:#fff;min-height:100vh;display:grid;place-items:center;margin:0}main{width:min(700px,90vw)}h1{font-size:72px;margin:0}form{display:flex;gap:10px}input,button{padding:14px;border-radius:10px;border:1px solid #333;font:inherit}input{flex:1;background:#14181d;color:#fff}button{font-weight:700}</style></head><body><main><h1>qh8z</h1><p>Fast links. Useful analytics. Built to become a real commercial shortener.</p><form id="f"><input id="u" type="url" placeholder="https://example.com/long-link" required><button>Shorten</button></form><p id="r"></p></main><script>f.onsubmit=async(e)=>{e.preventDefault();r.textContent='Shortening...';try{let x=await fetch('/api/v1/links',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({url:u.value})}),d=await x.json();if(!x.ok)throw Error(d.error);r.innerHTML='<a style="color:white" href="'+d.shortUrl+'">'+d.shortUrl+'</a>'}catch(x){r.textContent=x.message}}</script></body></html>`))
}

func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw)); if err != nil || u.Host == "" { return "", errors.New("invalid URL") }
	if u.Scheme != "http" && u.Scheme != "https" { return "", errors.New("URL must use http or https") }
	if u.User != nil { return "", errors.New("URLs containing credentials are not allowed") }
	return u.String(), nil
}

func randomSlug(n int) (string, error) {
	out := make([]byte, 0, n); b := make([]byte, 1)
	for len(out) < n { if _, err := rand.Read(b); err != nil { return "", err }; if b[0] >= 252 { continue }; out = append(out, alphabet[int(b[0])%len(alphabet)]) }
	return string(out), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) { w.Header().Set("Content-Type", "application/json; charset=utf-8"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(value) }
func writeError(w http.ResponseWriter, status int, msg string) { writeJSON(w, status, map[string]string{"error":msg}) }
func envOr(k, fallback string) string { if v := os.Getenv(k); v != "" { return v }; return fallback }
func securityHeaders(next http.Handler) http.Handler { return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("X-Content-Type-Options", "nosniff"); w.Header().Set("X-Frame-Options", "DENY"); w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin"); next.ServeHTTP(w,r) }) }

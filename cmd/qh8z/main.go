package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/storage"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/storage/postgres"
)

type app struct {
	store   storage.Store
	baseURL string
	logger  *slog.Logger
}

type linkResponse struct {
	Slug      string    `json:"slug"`
	URL       string    `json:"url"`
	ShortURL  string    `json:"shortUrl"`
	CreatedAt time.Time `json:"createdAt"`
	Visits    int64     `json:"visits"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9_-]{3,64}$`)
var reserved = map[string]bool{"api": true, "healthz": true, "readyz": true, "admin": true, "login": true, "signup": true, "pricing": true}

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	port := envOr("PORT", "8080")
	baseURL := strings.TrimRight(envOr("QH8Z_BASE_URL", "http://localhost:"+port), "/")
	store, err := openStore(context.Background(), logger)
	if err != nil {
		logger.Error("storage startup failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	a := &app{store: store, baseURL: baseURL, logger: logger}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           securityHeaders(a.routes()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("http shutdown failed", "error", err)
		}
	}()

	logger.Info("qh8z listening", "address", server.Addr, "base_url", baseURL)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}

func openStore(ctx context.Context, logger *slog.Logger) (storage.Store, error) {
	mode := strings.ToLower(envOr("QH8Z_STORAGE", "memory"))
	environment := strings.ToLower(envOr("QH8Z_ENV", "development"))
	if environment == "production" && mode != "postgres" {
		return nil, errors.New("QH8Z_STORAGE must be postgres when QH8Z_ENV=production")
	}
	switch mode {
	case "memory":
		logger.Warn("using in-memory storage; data will not survive restarts")
		return storage.NewMemory(), nil
	case "postgres":
		return postgres.Open(ctx, os.Getenv("DATABASE_URL"))
	default:
		return nil, errors.New("QH8Z_STORAGE must be memory or postgres")
	}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /readyz", a.ready)
	mux.HandleFunc("POST /api/v1/links", a.create)
	mux.HandleFunc("GET /api/v1/links/{slug}", a.get)
	mux.HandleFunc("GET /api/v1/links/{slug}/stats", a.stats)
	mux.HandleFunc("GET /{slug}", a.redirect)
	mux.HandleFunc("GET /", a.home)
	return mux
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.store.Ping(ctx); err != nil {
		a.logger.Error("readiness check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *app) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL        string `json:"url"`
		CustomSlug string `json:"customSlug"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}
	target, err := normalizeURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	custom := strings.TrimSpace(req.CustomSlug)
	if custom != "" {
		if !slugPattern.MatchString(custom) || reserved[custom] {
			writeError(w, http.StatusBadRequest, "custom slug must be 3-64 lowercase letters, numbers, hyphens, or underscores and cannot be reserved")
			return
		}
		item := core.Link{Slug: custom, URL: target, CreatedAt: time.Now().UTC()}
		if err := a.store.CreateLink(r.Context(), item); err != nil {
			a.writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, a.response(item))
		return
	}

	for attempts := 0; attempts < 8; attempts++ {
		slug, err := randomSlug(7)
		if err != nil {
			a.logger.Error("slug generation failed", "error", err)
			writeError(w, http.StatusInternalServerError, "could not generate slug")
			return
		}
		item := core.Link{Slug: slug, URL: target, CreatedAt: time.Now().UTC()}
		err = a.store.CreateLink(r.Context(), item)
		if errors.Is(err, core.ErrConflict) {
			continue
		}
		if err != nil {
			a.writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, a.response(item))
		return
	}
	writeError(w, http.StatusServiceUnavailable, "could not allocate a unique short code")
}

func (a *app) get(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.GetLink(r.Context(), r.PathValue("slug"))
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.response(item))
}

func (a *app) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.Stats(r.Context(), r.PathValue("slug"), 50)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *app) redirect(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	item, err := a.store.GetLink(r.Context(), slug)
	if errors.Is(err, core.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.logger.Error("redirect lookup failed", "slug", slug, "error", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	visit := core.Visit{
		Slug:      slug,
		VisitedAt: time.Now().UTC(),
		Referer:   sanitizeHeader(r.Referer(), 2048),
		UserAgent: sanitizeHeader(r.UserAgent(), 1024),
	}
	if _, err := a.store.RecordVisit(r.Context(), visit); err != nil {
		// Link delivery is more important than analytics availability. The future async
		// analytics pipeline will remove this best-effort write from the redirect path.
		a.logger.Error("visit recording failed", "slug", slug, "error", err)
	}
	http.Redirect(w, r, item.URL, http.StatusFound)
}

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>qh8z</title><style>body{font-family:system-ui,sans-serif;background:#0b0d10;color:#fff;min-height:100vh;display:grid;place-items:center;margin:0}main{width:min(700px,90vw)}h1{font-size:72px;margin:0}form{display:flex;gap:10px}input,button{padding:14px;border-radius:10px;border:1px solid #333;font:inherit}input{flex:1;background:#14181d;color:#fff}button{font-weight:700}a{color:#fff}</style></head><body><main><h1>qh8z</h1><p>Fast links. Durable analytics. Built to become a real commercial shortener.</p><form id="f"><input id="u" type="url" placeholder="https://example.com/long-link" required><button>Shorten</button></form><p id="r"></p></main><script>f.onsubmit=async(e)=>{e.preventDefault();r.textContent='Shortening...';try{const x=await fetch('/api/v1/links',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({url:u.value})});const d=await x.json();if(!x.ok)throw Error(d.error);const a=document.createElement('a');a.href=d.shortUrl;a.textContent=d.shortUrl;r.replaceChildren(a)}catch(x){r.textContent=x.message}}</script></body></html>`))
}

func (a *app) response(item core.Link) linkResponse {
	return linkResponse{
		Slug:      item.Slug,
		URL:       item.URL,
		ShortURL:  a.baseURL + "/" + item.Slug,
		CreatedAt: item.CreatedAt,
		Visits:    item.Visits,
	}
}

func (a *app) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, "link not found")
	case errors.Is(err, core.ErrConflict):
		writeError(w, http.StatusConflict, "slug already exists")
	default:
		a.logger.Error("storage request failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
	}
}

func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", errors.New("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("URL must use http or https")
	}
	if u.User != nil {
		return "", errors.New("URLs containing credentials are not allowed")
	}
	return u.String(), nil
}

func randomSlug(n int) (string, error) {
	out := make([]byte, 0, n)
	b := make([]byte, 1)
	for len(out) < n {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		if b[0] >= 252 {
			continue
		}
		out = append(out, alphabet[int(b[0])%len(alphabet)])
	}
	return string(out), nil
}

func sanitizeHeader(value string, maxBytes int) string {
	value = strings.ToValidUTF8(value, "")
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

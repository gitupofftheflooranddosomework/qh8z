package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/dnsverify"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/mailer"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/reputation"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/storage"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/storage/postgres"
)

type app struct {
	store         storage.Store
	mailer        mailer.Mailer
	reputation    reputation.Checker
	dnsVerifier   dnsverify.Verifier
	safety        safetyConfig
	baseURL       string
	logger        *slog.Logger
	secureCookies bool
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	port := envOr("PORT", "8080")
	baseURL := strings.TrimRight(envOr("QH8Z_BASE_URL", "http://localhost:"+port), "/")
	environment := strings.ToLower(envOr("QH8Z_ENV", "development"))
	if environment == "production" && !strings.HasPrefix(baseURL, "https://") {
		logger.Error("QH8Z_BASE_URL must use https in production")
		os.Exit(1)
	}

	store, err := openStore(context.Background(), logger, environment)
	if err != nil {
		logger.Error("storage startup failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	email, err := openMailer(logger, environment)
	if err != nil {
		logger.Error("email startup failed", "error", err)
		os.Exit(1)
	}

	checker, safety, err := openSafety(environment)
	if err != nil {
		logger.Error("safety startup failed", "error", err)
		os.Exit(1)
	}
	if err := configureBilling(environment); err != nil {
		logger.Error("billing startup failed", "error", err)
		os.Exit(1)
	}

	a := &app{
		store:         store,
		mailer:        email,
		reputation:    checker,
		dnsVerifier:   dnsverify.System{},
		safety:        safety,
		baseURL:       baseURL,
		logger:        logger,
		secureCookies: environment == "production",
	}
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

	logger.Info("qh8z listening", "address", server.Addr, "base_url", baseURL, "environment", environment)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}

func openStore(ctx context.Context, logger *slog.Logger, environment string) (storage.Store, error) {
	mode := strings.ToLower(envOr("QH8Z_STORAGE", "memory"))
	if environment == "production" && mode != "postgres" {
		return nil, errors.New("QH8Z_STORAGE must be postgres when QH8Z_ENV=production")
	}
	switch mode {
	case "memory":
		logger.Warn("using in-memory storage; data will not survive restarts")
		return storage.NewMemory(), nil
	case "postgres":
		dsn, err := databaseURL()
		if err != nil {
			return nil, err
		}
		return postgres.Open(ctx, dsn)
	default:
		return nil, errors.New("QH8Z_STORAGE must be memory or postgres")
	}
}

func openMailer(logger *slog.Logger, environment string) (mailer.Mailer, error) {
	mode := strings.ToLower(envOr("QH8Z_EMAIL_MODE", "log"))
	if environment == "production" && mode != "smtp" {
		return nil, errors.New("QH8Z_EMAIL_MODE must be smtp when QH8Z_ENV=production")
	}
	switch mode {
	case "log":
		return mailer.Log{Logger: logger}, nil
	case "smtp":
		username, err := secretValue("SMTP_USERNAME")
		if err != nil {
			return nil, err
		}
		password, err := secretValue("SMTP_PASSWORD")
		if err != nil {
			return nil, err
		}
		cfg := mailer.SMTPConfig{
			Address:  envOr("SMTP_ADDR", ""),
			Host:     envOr("SMTP_HOST", ""),
			Username: username,
			Password: password,
			From:     envOr("SMTP_FROM", ""),
		}
		if cfg.Address == "" || cfg.Host == "" || cfg.From == "" {
			return nil, errors.New("SMTP_ADDR, SMTP_HOST, and SMTP_FROM are required for SMTP email")
		}
		return mailer.SMTP{Config: cfg}, nil
	default:
		return nil, errors.New("QH8Z_EMAIL_MODE must be log or smtp")
	}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /readyz", a.ready)
	mux.HandleFunc("GET /metrics", a.metrics)
	mux.HandleFunc("GET /internal/tls/allow", a.tlsAllow)
	mux.HandleFunc("GET /dashboard", a.dashboard)
	mux.HandleFunc("GET /assets/dashboard.js", a.dashboardJS)
	mux.HandleFunc("GET /terms", a.termsPage)
	mux.HandleFunc("GET /privacy", a.privacyPage)
	mux.HandleFunc("GET /acceptable-use", a.acceptableUsePage)

	mux.Handle("POST /api/v1/auth/register", a.limitIPHandler("register", 10, time.Hour, http.HandlerFunc(a.register)))
	mux.Handle("POST /api/v1/auth/login", a.limitIPHandler("login", 60, 15*time.Minute, http.HandlerFunc(a.login)))
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.HandleFunc("POST /api/v1/auth/verify-email", a.verifyEmail)
	mux.HandleFunc("POST /api/v1/auth/resend-verification", a.resendVerification)
	mux.HandleFunc("GET /api/v1/me", a.me)
	mux.HandleFunc("GET /verify-email", a.verifyEmailPage)

	mux.HandleFunc("GET /api/v1/workspaces", a.listWorkspaces)
	mux.HandleFunc("POST /api/v1/workspaces", a.createWorkspace)
	mux.HandleFunc("GET /api/v1/workspaces/{workspace}/members", a.listWorkspaceMembers)
	mux.HandleFunc("POST /api/v1/workspaces/{workspace}/members", a.addWorkspaceMember)
	mux.HandleFunc("POST /api/v1/workspaces/{workspace}/api-keys", a.createAPIKey)
	mux.HandleFunc("GET /api/v1/workspaces/{workspace}/audit", a.auditLog)

	mux.HandleFunc("POST /api/v1/abuse-reports", a.createAbuseReport)
	mux.HandleFunc("GET /api/v1/admin/abuse-reports", a.listAbuseReports)
	mux.HandleFunc("PATCH /api/v1/admin/abuse-reports/{id}", a.reviewAbuseReport)
	mux.HandleFunc("GET /api/v1/admin/url-rules", a.listURLRules)
	mux.HandleFunc("POST /api/v1/admin/url-rules", a.createURLRule)
	mux.HandleFunc("DELETE /api/v1/admin/url-rules/{id}", a.deleteURLRule)
	mux.HandleFunc("POST /api/v1/admin/links/{slug}/suspend", a.suspendLink)
	mux.HandleFunc("POST /api/v1/admin/links/{slug}/unsuspend", a.unsuspendLink)

	mux.HandleFunc("GET /api/v1/plans", a.listPlans)
	mux.HandleFunc("GET /api/v1/usage", a.usage)
	mux.HandleFunc("GET /api/v1/analytics", a.analyticsDashboard)
	mux.HandleFunc("GET /api/v1/billing", a.billingStatus)
	mux.HandleFunc("POST /api/v1/billing/checkout", a.createCheckout)
	mux.HandleFunc("POST /api/v1/billing/portal", a.createBillingPortal)
	mux.HandleFunc("POST /api/v1/billing/webhook", a.billingWebhook)
	mux.HandleFunc("GET /api/v1/domains", a.listCustomDomains)
	mux.HandleFunc("POST /api/v1/domains", a.createCustomDomain)
	mux.HandleFunc("POST /api/v1/domains/{id}/verify", a.verifyCustomDomain)
	mux.HandleFunc("DELETE /api/v1/domains/{id}", a.deleteCustomDomain)

	mux.HandleFunc("GET /api/v1/links", a.listLinks)
	mux.HandleFunc("POST /api/v1/links", a.createLink)
	mux.HandleFunc("GET /api/v1/links/{slug}", a.getLink)
	mux.HandleFunc("PATCH /api/v1/links/{slug}", a.updateLink)
	mux.HandleFunc("DELETE /api/v1/links/{slug}", a.deleteLink)
	mux.HandleFunc("GET /api/v1/links/{slug}/stats", a.linkStats)
	mux.HandleFunc("GET /api/v1/links/{slug}/qr.png", a.linkQRCode)
	mux.HandleFunc("GET /{slug}", a.redirect)
	mux.HandleFunc("GET /", a.home)

	limited := a.apiRateLimit(mux)
	routed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/billing/webhook" {
			mux.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
	return a.observe(routed)
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

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>qh8z</title><style>body{font-family:system-ui,sans-serif;background:#0b0d10;color:#fff;min-height:100vh;display:grid;place-items:center;margin:0}main{width:min(720px,90vw)}h1{font-size:72px;margin:0}a{color:white}code{background:#181c21;padding:.2em .4em;border-radius:6px}.legal{margin-top:28px;color:#9da8b7;font-size:14px}.legal a{color:#c7d0dc}</style></head><body><main><h1>qh8z</h1><p>Fast links. Durable analytics. Secure accounts and workspace ownership.</p><p><a href="/dashboard">Open dashboard</a> · API available under <code>/api/v1</code>.</p><p class="legal"><a href="/terms">Terms</a> · <a href="/privacy">Privacy</a> · <a href="/acceptable-use">Acceptable Use</a> · <a href="mailto:abuse@qh8z.com">Report abuse</a></p></main></body></html>`))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

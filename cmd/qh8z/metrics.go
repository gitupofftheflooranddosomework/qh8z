package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

type metricsState struct {
	startedAt     time.Time
	requests      atomic.Uint64
	active        atomic.Int64
	status5xx     atomic.Uint64
	rateLimited   atomic.Uint64
	redirects     atomic.Uint64
	durationNanos atomic.Uint64
}

var serviceMetrics = &metricsState{startedAt: time.Now().UTC()}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (a *app) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		serviceMetrics.requests.Add(1)
		serviceMetrics.active.Add(1)
		defer serviceMetrics.active.Add(-1)
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		if status >= 500 {
			serviceMetrics.status5xx.Add(1)
		}
		if status == http.StatusTooManyRequests {
			serviceMetrics.rateLimited.Add(1)
		}
		if status >= 300 && status < 400 {
			serviceMetrics.redirects.Add(1)
		}
		serviceMetrics.durationNanos.Add(uint64(time.Since(started)))
	})
}

func (a *app) metrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	storageUp := 1
	if err := a.store.Ping(ctx); err != nil {
		storageUp = 0
	}
	requests := serviceMetrics.requests.Load()
	durationSeconds := float64(serviceMetrics.durationNanos.Load()) / float64(time.Second)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	writeMetric := func(name, help, metricType, value string) {
		_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %s\n", name, help, name, metricType, name, value)
	}
	writeMetric("qh8z_uptime_seconds", "Seconds since the qh8z process started.", "gauge", strconv.FormatFloat(time.Since(serviceMetrics.startedAt).Seconds(), 'f', 3, 64))
	writeMetric("qh8z_http_requests_total", "HTTP requests observed by qh8z.", "counter", strconv.FormatUint(requests, 10))
	writeMetric("qh8z_http_active_requests", "HTTP requests currently being served.", "gauge", strconv.FormatInt(serviceMetrics.active.Load(), 10))
	writeMetric("qh8z_http_5xx_total", "HTTP responses with a 5xx status.", "counter", strconv.FormatUint(serviceMetrics.status5xx.Load(), 10))
	writeMetric("qh8z_http_rate_limited_total", "HTTP responses rejected with status 429.", "counter", strconv.FormatUint(serviceMetrics.rateLimited.Load(), 10))
	writeMetric("qh8z_http_redirects_total", "HTTP redirect responses.", "counter", strconv.FormatUint(serviceMetrics.redirects.Load(), 10))
	writeMetric("qh8z_http_request_duration_seconds_sum", "Cumulative request duration in seconds.", "counter", strconv.FormatFloat(durationSeconds, 'f', 6, 64))
	writeMetric("qh8z_http_request_duration_seconds_count", "Count of request durations.", "counter", strconv.FormatUint(requests, 10))
	writeMetric("qh8z_storage_up", "Whether the configured storage backend is reachable.", "gauge", strconv.Itoa(storageUp))
}

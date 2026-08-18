package reputation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const defaultWebRiskEndpoint = "https://webrisk.googleapis.com/v1/uris:search"

type cachedResult struct {
	result    Result
	expiresAt time.Time
}

type WebRisk struct {
	APIKey           string
	Endpoint         string
	Client           *http.Client
	ExtendedCoverage bool

	mu    sync.Mutex
	cache map[string]cachedResult
}

func NewWebRisk(apiKey string, extendedCoverage bool) (*WebRisk, error) {
	if apiKey == "" {
		return nil, errors.New("WEBRISK_API_KEY is required")
	}
	return &WebRisk{
		APIKey:           apiKey,
		Endpoint:         defaultWebRiskEndpoint,
		Client:           &http.Client{Timeout: 5 * time.Second},
		ExtendedCoverage: extendedCoverage,
		cache:            make(map[string]cachedResult),
	}, nil
}

func (w *WebRisk) Check(ctx context.Context, target string) (Result, error) {
	now := time.Now()
	w.mu.Lock()
	if cached, ok := w.cache[target]; ok && cached.expiresAt.After(now) {
		w.mu.Unlock()
		return cached.result, nil
	}
	w.mu.Unlock()

	endpoint := w.Endpoint
	if endpoint == "" {
		endpoint = defaultWebRiskEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return Result{}, fmt.Errorf("parse Web Risk endpoint: %w", err)
	}
	query := parsed.Query()
	query.Set("uri", target)
	query.Set("key", w.APIKey)
	query.Add("threatTypes", "MALWARE")
	query.Add("threatTypes", "SOCIAL_ENGINEERING")
	query.Add("threatTypes", "UNWANTED_SOFTWARE")
	if w.ExtendedCoverage {
		query.Add("threatTypes", "SOCIAL_ENGINEERING_EXTENDED_COVERAGE")
	}
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("create Web Risk request: %w", err)
	}
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// Do not include the underlying error string here: HTTP client errors can
		// contain the request URL, and the Web Risk API key is a query parameter.
		return Result{}, fmt.Errorf("Web Risk request failed (%T)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return Result{}, fmt.Errorf("Web Risk returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Threat *struct {
			ThreatTypes []string  `json:"threatTypes"`
			ExpireTime  time.Time `json:"expireTime"`
		} `json:"threat"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		return Result{}, fmt.Errorf("decode Web Risk response: %w", err)
	}

	result := Result{}
	cacheUntil := now.Add(5 * time.Minute)
	if payload.Threat != nil && len(payload.Threat.ThreatTypes) > 0 {
		result.Unsafe = true
		result.ThreatTypes = append([]string(nil), payload.Threat.ThreatTypes...)
		result.ExpiresAt = payload.Threat.ExpireTime
		if payload.Threat.ExpireTime.After(now) {
			cacheUntil = payload.Threat.ExpireTime
		}
	}
	w.mu.Lock()
	if w.cache == nil {
		w.cache = make(map[string]cachedResult)
	}
	if len(w.cache) >= 10000 {
		// Keep the per-process optimization bounded. PostgreSQL remains qh8z's
		// durable source of truth; reputation cache misses are safe to re-check.
		w.cache = make(map[string]cachedResult)
	}
	w.cache[target] = cachedResult{result: result, expiresAt: cacheUntil}
	w.mu.Unlock()
	return result, nil
}

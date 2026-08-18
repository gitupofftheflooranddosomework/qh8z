package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("billing is unavailable")

type CheckoutRequest struct {
	WorkspaceID string
	Email       string
	CustomerID  string
	SuccessURL  string
	CancelURL   string
}

type Session struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type Event struct {
	ID   string
	Type string
	Data json.RawMessage
}

type Provider interface {
	Checkout(context.Context, CheckoutRequest) (Session, error)
	Portal(context.Context, string, string) (Session, error)
	VerifyWebhook([]byte, string, time.Time) (Event, error)
}

type Disabled struct{}

func (Disabled) Checkout(context.Context, CheckoutRequest) (Session, error) {
	return Session{}, ErrUnavailable
}

func (Disabled) Portal(context.Context, string, string) (Session, error) {
	return Session{}, ErrUnavailable
}

func (Disabled) VerifyWebhook([]byte, string, time.Time) (Event, error) {
	return Event{}, ErrUnavailable
}

type Stripe struct {
	SecretKey     string
	WebhookSecret string
	ProPriceID    string
	APIBase       string
	HTTPClient    *http.Client
}

func (s Stripe) Checkout(ctx context.Context, req CheckoutRequest) (Session, error) {
	if s.SecretKey == "" || s.ProPriceID == "" {
		return Session{}, ErrUnavailable
	}
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", s.ProPriceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", req.SuccessURL)
	form.Set("cancel_url", req.CancelURL)
	form.Set("client_reference_id", req.WorkspaceID)
	form.Set("metadata[workspace_id]", req.WorkspaceID)
	form.Set("subscription_data[metadata][workspace_id]", req.WorkspaceID)
	if req.CustomerID != "" {
		form.Set("customer", req.CustomerID)
	} else if req.Email != "" {
		form.Set("customer_email", req.Email)
	}
	var response struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := s.postForm(ctx, "/v1/checkout/sessions", form, &response); err != nil {
		return Session{}, err
	}
	if response.ID == "" || response.URL == "" {
		return Session{}, errors.New("Stripe returned an incomplete Checkout Session")
	}
	return Session{ID: response.ID, URL: response.URL}, nil
}

func (s Stripe) Portal(ctx context.Context, customerID, returnURL string) (Session, error) {
	if s.SecretKey == "" || customerID == "" {
		return Session{}, ErrUnavailable
	}
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", returnURL)
	var response struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := s.postForm(ctx, "/v1/billing_portal/sessions", form, &response); err != nil {
		return Session{}, err
	}
	if response.ID == "" || response.URL == "" {
		return Session{}, errors.New("Stripe returned an incomplete portal Session")
	}
	return Session{ID: response.ID, URL: response.URL}, nil
}

func (s Stripe) VerifyWebhook(payload []byte, signatureHeader string, now time.Time) (Event, error) {
	if s.WebhookSecret == "" {
		return Event{}, ErrUnavailable
	}
	timestamp, signatures, err := parseSignatureHeader(signatureHeader)
	if err != nil {
		return Event{}, err
	}
	if delta := now.Unix() - timestamp; delta > 300 || delta < -300 {
		return Event{}, errors.New("Stripe webhook timestamp is outside the allowed tolerance")
	}
	signedPayload := strconv.FormatInt(timestamp, 10) + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	_, _ = mac.Write([]byte(signedPayload))
	expected := mac.Sum(nil)
	valid := false
	for _, signature := range signatures {
		decoded, err := hex.DecodeString(signature)
		if err == nil && hmac.Equal(decoded, expected) {
			valid = true
			break
		}
	}
	if !valid {
		return Event{}, errors.New("invalid Stripe webhook signature")
	}
	var envelope struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return Event{}, fmt.Errorf("decode Stripe webhook: %w", err)
	}
	if envelope.ID == "" || envelope.Type == "" || len(envelope.Data.Object) == 0 {
		return Event{}, errors.New("Stripe webhook is missing required fields")
	}
	return Event{ID: envelope.ID, Type: envelope.Type, Data: envelope.Data.Object}, nil
}

func (s Stripe) postForm(ctx context.Context, path string, form url.Values, out any) error {
	base := strings.TrimRight(s.APIBase, "/")
	if base == "" {
		base = "https://api.stripe.com"
	}
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create Stripe request: %w", err)
	}
	req.SetBasicAuth(s.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Stripe request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return fmt.Errorf("Stripe API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("decode Stripe response: %w", err)
	}
	return nil
}

func parseSignatureHeader(header string) (int64, []string, error) {
	var timestamp int64
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, nil, errors.New("invalid Stripe webhook timestamp")
			}
			timestamp = parsed
		case "v1":
			if value != "" {
				signatures = append(signatures, value)
			}
		}
	}
	if timestamp == 0 || len(signatures) == 0 {
		return 0, nil, errors.New("Stripe-Signature is missing timestamp or v1 signature")
	}
	return timestamp, signatures, nil
}

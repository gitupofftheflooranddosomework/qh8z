package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStripeCheckoutAndPortal(t *testing.T) {
	var checkoutForm, portalForm map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, _, ok := r.BasicAuth(); !ok || user != "sk_test_qh8z" {
			t.Errorf("unexpected Stripe auth: %q ok=%v", user, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		switch r.URL.Path {
		case "/v1/checkout/sessions":
			checkoutForm = r.PostForm
			_, _ = w.Write([]byte(`{"id":"cs_test_1","url":"https://checkout.stripe.test/session"}`))
		case "/v1/billing_portal/sessions":
			portalForm = r.PostForm
			_, _ = w.Write([]byte(`{"id":"bps_test_1","url":"https://billing.stripe.test/portal"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stripe := Stripe{SecretKey: "sk_test_qh8z", ProPriceID: "price_pro", APIBase: server.URL, HTTPClient: server.Client()}
	checkout, err := stripe.Checkout(context.Background(), CheckoutRequest{
		WorkspaceID: "ws_123",
		Email:       "owner@example.com",
		SuccessURL:  "https://qh8z.test/dashboard?checkout=success",
		CancelURL:   "https://qh8z.test/dashboard?checkout=canceled",
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if checkout.ID != "cs_test_1" || checkout.URL == "" {
		t.Fatalf("checkout session = %+v", checkout)
	}
	if checkoutForm.Get("mode") != "subscription" || checkoutForm.Get("line_items[0][price]") != "price_pro" || checkoutForm.Get("client_reference_id") != "ws_123" || checkoutForm.Get("subscription_data[metadata][workspace_id]") != "ws_123" {
		t.Fatalf("checkout form = %#v", checkoutForm)
	}

	portal, err := stripe.Portal(context.Background(), "cus_123", "https://qh8z.test/dashboard")
	if err != nil {
		t.Fatalf("portal: %v", err)
	}
	if portal.ID != "bps_test_1" || portal.URL == "" {
		t.Fatalf("portal session = %+v", portal)
	}
	if portalForm.Get("customer") != "cus_123" || portalForm.Get("return_url") != "https://qh8z.test/dashboard" {
		t.Fatalf("portal form = %#v", portalForm)
	}
}

func TestStripeWebhookVerification(t *testing.T) {
	secret := "whsec_test"
	now := time.Unix(1_800_000_000, 0).UTC()
	payload := []byte(`{"id":"evt_123","type":"checkout.session.completed","data":{"object":{"client_reference_id":"ws_123"}}}`)
	signed := strconv.FormatInt(now.Unix(), 10) + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signed))
	hexSignature := hex.EncodeToString(mac.Sum(nil))
	header := "t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=" + hexSignature

	stripe := Stripe{WebhookSecret: secret}
	event, err := stripe.VerifyWebhook(payload, header, now)
	if err != nil {
		t.Fatalf("verify webhook: %v", err)
	}
	if event.ID != "evt_123" || event.Type != "checkout.session.completed" || !strings.Contains(string(event.Data), "ws_123") {
		t.Fatalf("event = %+v", event)
	}
	if _, err := stripe.VerifyWebhook(payload, header+"bad", now); err == nil {
		t.Fatal("expected invalid signature to fail")
	}
	if _, err := stripe.VerifyWebhook(payload, header, now.Add(10*time.Minute)); err == nil {
		t.Fatal("expected stale webhook to fail")
	}
}

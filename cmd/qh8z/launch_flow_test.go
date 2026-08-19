package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/billing"
)

func TestLaunchJourneySignupShortenRedirectAnalyticsAndBilling(t *testing.T) {
	const (
		stripeSecret  = "sk_test_qh8z_launch_acceptance"
		webhookSecret = "whsec_qh8z_launch_acceptance"
		priceID       = "price_qh8z_pro_launch"
	)

	var checkoutWorkspace string
	stripeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/checkout/sessions" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse Stripe checkout form: %v", err)
		}
		if got := r.Form.Get("line_items[0][price]"); got != priceID {
			t.Errorf("checkout price = %q, want %q", got, priceID)
		}
		checkoutWorkspace = r.Form.Get("metadata[workspace_id]")
		if checkoutWorkspace == "" || r.Form.Get("client_reference_id") != checkoutWorkspace {
			t.Errorf("checkout workspace metadata = %q, client reference = %q", checkoutWorkspace, r.Form.Get("client_reference_id"))
		}
		if r.Form.Get("success_url") != "https://qh8z.test/dashboard?checkout=success" {
			t.Errorf("checkout success URL = %q", r.Form.Get("success_url"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cs_test_launch","url":"https://checkout.stripe.test/cs_test_launch"}`))
	}))
	defer stripeAPI.Close()

	previousProvider := billingProvider
	billingProvider = billing.Stripe{
		SecretKey:     stripeSecret,
		WebhookSecret: webhookSecret,
		ProPriceID:    priceID,
		APIBase:       stripeAPI.URL,
		HTTPClient:    stripeAPI.Client(),
	}
	defer func() { billingProvider = previousProvider }()

	a, fm := testApp()
	owner, cookie := registerAndVerify(t, a, fm, "launch-journey@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com/launch-destination","customSlug":"launch-journey"}`))
	createReq.AddCookie(cookie)
	createReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	createRec := httptest.NewRecorder()
	a.routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("shorten status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "https://qh8z.test/launch-journey", nil)
	redirectReq.Header.Set("Referer", "https://launch-referrer.example/path")
	redirectRec := httptest.NewRecorder()
	a.routes().ServeHTTP(redirectRec, redirectReq)
	if redirectRec.Code != http.StatusFound || redirectRec.Header().Get("Location") != "https://example.com/launch-destination" {
		t.Fatalf("redirect = %d %q", redirectRec.Code, redirectRec.Header().Get("Location"))
	}

	analyticsReq := httptest.NewRequest(http.MethodGet, "/api/v1/analytics?days=7", nil)
	analyticsReq.AddCookie(cookie)
	analyticsReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	analyticsRec := httptest.NewRecorder()
	a.routes().ServeHTTP(analyticsRec, analyticsReq)
	if analyticsRec.Code != http.StatusOK || !strings.Contains(analyticsRec.Body.String(), "launch-journey") || !strings.Contains(analyticsRec.Body.String(), "launch-referrer.example") {
		t.Fatalf("analytics status = %d, body = %s", analyticsRec.Code, analyticsRec.Body.String())
	}

	qrReq := httptest.NewRequest(http.MethodGet, "/api/v1/links/launch-journey/qr.png?size=128", nil)
	qrReq.AddCookie(cookie)
	qrReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	qrRec := httptest.NewRecorder()
	a.routes().ServeHTTP(qrRec, qrReq)
	if qrRec.Code != http.StatusOK || qrRec.Header().Get("Content-Type") != "image/png" || qrRec.Body.Len() < 100 {
		t.Fatalf("QR response status=%d type=%q bytes=%d", qrRec.Code, qrRec.Header().Get("Content-Type"), qrRec.Body.Len())
	}

	checkoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/billing/checkout", nil)
	checkoutReq.AddCookie(cookie)
	checkoutReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	checkoutRec := httptest.NewRecorder()
	a.routes().ServeHTTP(checkoutRec, checkoutReq)
	if checkoutRec.Code != http.StatusCreated || !strings.Contains(checkoutRec.Body.String(), "cs_test_launch") {
		t.Fatalf("checkout status = %d, body = %s", checkoutRec.Code, checkoutRec.Body.String())
	}
	if checkoutWorkspace != owner.Workspace.ID {
		t.Fatalf("checkout workspace = %q, want %q", checkoutWorkspace, owner.Workspace.ID)
	}

	checkoutWebhook := fmt.Sprintf(`{"id":"evt_launch_checkout","type":"checkout.session.completed","data":{"object":{"client_reference_id":%q,"customer":"cus_launch","subscription":"sub_launch","metadata":{"workspace_id":%q}}}}`, owner.Workspace.ID, owner.Workspace.ID)
	postStripeWebhook(t, a, webhookSecret, checkoutWebhook)

	billingReq := httptest.NewRequest(http.MethodGet, "/api/v1/billing", nil)
	billingReq.AddCookie(cookie)
	billingReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	billingRec := httptest.NewRecorder()
	a.routes().ServeHTTP(billingRec, billingReq)
	if billingRec.Code != http.StatusOK || !strings.Contains(billingRec.Body.String(), `"planCode":"pro"`) || !strings.Contains(billingRec.Body.String(), `"customDomainLimit":10`) {
		t.Fatalf("Pro billing status = %d, body = %s", billingRec.Code, billingRec.Body.String())
	}

	cancelWebhook := fmt.Sprintf(`{"id":"evt_launch_cancel","type":"customer.subscription.deleted","data":{"object":{"id":"sub_launch","customer":"cus_launch","status":"canceled","metadata":{"workspace_id":%q}}}}`, owner.Workspace.ID)
	postStripeWebhook(t, a, webhookSecret, cancelWebhook)

	canceledReq := httptest.NewRequest(http.MethodGet, "/api/v1/billing", nil)
	canceledReq.AddCookie(cookie)
	canceledReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	canceledRec := httptest.NewRecorder()
	a.routes().ServeHTTP(canceledRec, canceledReq)
	if canceledRec.Code != http.StatusOK || !strings.Contains(canceledRec.Body.String(), `"status":"canceled"`) || !strings.Contains(canceledRec.Body.String(), `"customDomainLimit":0`) {
		t.Fatalf("canceled billing status = %d, body = %s", canceledRec.Code, canceledRec.Body.String())
	}
}

func postStripeWebhook(t *testing.T, a *app, secret, payload string) {
	t.Helper()
	timestamp := time.Now().UTC().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.%s", timestamp, payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/webhook", strings.NewReader(payload))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, signature))
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"processed"`) {
		t.Fatalf("Stripe webhook status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

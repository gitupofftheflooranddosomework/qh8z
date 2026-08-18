package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type allowDNSVerifier struct{}

func (allowDNSVerifier) Verify(context.Context, string, string) (bool, error) { return true, nil }

func TestCommercialLinkManagementAnalyticsAndQR(t *testing.T) {
	a, fm := testApp()
	owner, cookie := registerAndVerify(t, a, fm, "commercial@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com/old","customSlug":"managed-link"}`))
	createReq.AddCookie(cookie)
	createReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	createRec := httptest.NewRecorder()
	a.routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/links", nil)
	listReq.AddCookie(cookie)
	listReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	listRec := httptest.NewRecorder()
	a.routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "managed-link") {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/links/managed-link", strings.NewReader(`{"url":"https://example.com/new","disabled":true}`))
	patchReq.AddCookie(cookie)
	patchReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	patchRec := httptest.NewRecorder()
	a.routes().ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK || !strings.Contains(patchRec.Body.String(), "disabledAt") {
		t.Fatalf("patch status = %d, body = %s", patchRec.Code, patchRec.Body.String())
	}

	disabledRedirect := httptest.NewRequest(http.MethodGet, "https://qh8z.test/managed-link", nil)
	disabledRec := httptest.NewRecorder()
	a.routes().ServeHTTP(disabledRec, disabledRedirect)
	if disabledRec.Code != http.StatusNotFound {
		t.Fatalf("disabled redirect status = %d", disabledRec.Code)
	}

	enableReq := httptest.NewRequest(http.MethodPatch, "/api/v1/links/managed-link", strings.NewReader(`{"disabled":false}`))
	enableReq.AddCookie(cookie)
	enableReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	enableRec := httptest.NewRecorder()
	a.routes().ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body = %s", enableRec.Code, enableRec.Body.String())
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "https://qh8z.test/managed-link", nil)
	redirectReq.Header.Set("Referer", "https://ref.example/")
	redirectRec := httptest.NewRecorder()
	a.routes().ServeHTTP(redirectRec, redirectReq)
	if redirectRec.Code != http.StatusFound || redirectRec.Header().Get("Location") != "https://example.com/new" {
		t.Fatalf("redirect = %d %q", redirectRec.Code, redirectRec.Header().Get("Location"))
	}

	analyticsReq := httptest.NewRequest(http.MethodGet, "/api/v1/analytics?days=7", nil)
	analyticsReq.AddCookie(cookie)
	analyticsReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	analyticsRec := httptest.NewRecorder()
	a.routes().ServeHTTP(analyticsRec, analyticsReq)
	if analyticsRec.Code != http.StatusOK || !strings.Contains(analyticsRec.Body.String(), "managed-link") || !strings.Contains(analyticsRec.Body.String(), "ref.example") {
		t.Fatalf("analytics status = %d, body = %s", analyticsRec.Code, analyticsRec.Body.String())
	}

	qrReq := httptest.NewRequest(http.MethodGet, "/api/v1/links/managed-link/qr.png?size=128", nil)
	qrReq.AddCookie(cookie)
	qrReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	qrRec := httptest.NewRecorder()
	a.routes().ServeHTTP(qrRec, qrReq)
	if qrRec.Code != http.StatusOK || qrRec.Header().Get("Content-Type") != "image/png" || qrRec.Body.Len() < 100 {
		t.Fatalf("QR response status=%d type=%q bytes=%d", qrRec.Code, qrRec.Header().Get("Content-Type"), qrRec.Body.Len())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/links/managed-link", nil)
	deleteReq.AddCookie(cookie)
	deleteReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	deleteRec := httptest.NewRecorder()
	a.routes().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestCustomDomainsRequireProAndRouteByHost(t *testing.T) {
	a, fm := testApp()
	a.dnsVerifier = allowDNSVerifier{}
	owner, cookie := registerAndVerify(t, a, fm, "domains@example.com")

	freeReq := httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(`{"host":"go.example.com"}`))
	freeReq.AddCookie(cookie)
	freeReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	freeRec := httptest.NewRecorder()
	a.routes().ServeHTTP(freeRec, freeReq)
	if freeRec.Code != http.StatusPaymentRequired {
		t.Fatalf("free domain status = %d, body = %s", freeRec.Code, freeRec.Body.String())
	}

	now := time.Now().UTC()
	if err := a.store.UpsertBillingState(context.Background(), core.BillingState{
		WorkspaceID: owner.Workspace.ID,
		PlanCode:    core.PlanPro,
		Status:      core.BillingStatusActive,
		UpdatedAt:   now,
	}, core.AuditEntry{}); err != nil {
		t.Fatalf("set Pro plan: %v", err)
	}

	createDomainReq := httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(`{"host":"go.example.com"}`))
	createDomainReq.AddCookie(cookie)
	createDomainReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	createDomainRec := httptest.NewRecorder()
	a.routes().ServeHTTP(createDomainRec, createDomainReq)
	if createDomainRec.Code != http.StatusCreated {
		t.Fatalf("create domain status = %d, body = %s", createDomainRec.Code, createDomainRec.Body.String())
	}
	var domainResult struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createDomainRec.Body).Decode(&domainResult); err != nil || domainResult.ID == "" {
		t.Fatalf("decode domain: %+v, err = %v", domainResult, err)
	}

	verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/domains/"+domainResult.ID+"/verify", nil)
	verifyReq.AddCookie(cookie)
	verifyReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	verifyRec := httptest.NewRecorder()
	a.routes().ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK || !strings.Contains(verifyRec.Body.String(), `"verified":true`) {
		t.Fatalf("verify domain status = %d, body = %s", verifyRec.Code, verifyRec.Body.String())
	}

	linkReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com/branded","customSlug":"brand","domainId":"`+domainResult.ID+`"}`))
	linkReq.AddCookie(cookie)
	linkReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	linkRec := httptest.NewRecorder()
	a.routes().ServeHTTP(linkRec, linkReq)
	if linkRec.Code != http.StatusCreated || !strings.Contains(linkRec.Body.String(), "https://go.example.com/brand") {
		t.Fatalf("branded link status = %d, body = %s", linkRec.Code, linkRec.Body.String())
	}

	primaryReq := httptest.NewRequest(http.MethodGet, "https://qh8z.test/brand", nil)
	primaryRec := httptest.NewRecorder()
	a.routes().ServeHTTP(primaryRec, primaryReq)
	if primaryRec.Code != http.StatusNotFound {
		t.Fatalf("primary host exposed branded slug with status %d", primaryRec.Code)
	}

	brandedReq := httptest.NewRequest(http.MethodGet, "https://go.example.com/brand", nil)
	brandedRec := httptest.NewRecorder()
	a.routes().ServeHTTP(brandedRec, brandedReq)
	if brandedRec.Code != http.StatusFound || brandedRec.Header().Get("Location") != "https://example.com/branded" {
		t.Fatalf("branded redirect = %d %q", brandedRec.Code, brandedRec.Header().Get("Location"))
	}
}

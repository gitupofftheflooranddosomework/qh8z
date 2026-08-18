package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/billing"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

var billingProvider billing.Provider = billing.Disabled{}

func configureBilling(environment string) error {
	mode := strings.ToLower(envOr("QH8Z_BILLING_MODE", "disabled"))
	if environment == "production" && mode != "stripe" {
		return errors.New("QH8Z_BILLING_MODE must be stripe when QH8Z_ENV=production")
	}
	switch mode {
	case "disabled":
		billingProvider = billing.Disabled{}
		return nil
	case "stripe":
		secretKey, err := secretValue("STRIPE_SECRET_KEY")
		if err != nil {
			return err
		}
		webhookSecret, err := secretValue("STRIPE_WEBHOOK_SECRET")
		if err != nil {
			return err
		}
		priceID := envOr("STRIPE_PRO_PRICE_ID", "")
		if secretKey == "" || webhookSecret == "" || priceID == "" {
			return errors.New("STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET, and STRIPE_PRO_PRICE_ID are required for Stripe billing")
		}
		billingProvider = billing.Stripe{
			SecretKey:     secretKey,
			WebhookSecret: webhookSecret,
			ProPriceID:    priceID,
		}
		return nil
	default:
		return errors.New("QH8Z_BILLING_MODE must be disabled or stripe")
	}
}

func (a *app) billingStatus(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:read", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	state, err := a.store.GetBillingState(r.Context(), auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	limits, err := a.workspacePlan(r, auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"billing": state, "limits": limits})
}

func (a *app) createCheckout(w http.ResponseWriter, r *http.Request) {
	auth, ok := a.billingAdmin(w, r)
	if !ok {
		return
	}
	state, err := a.store.GetBillingState(r.Context(), auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	if state.PlanCode == core.PlanPro && state.Status != core.BillingStatusCanceled {
		writeError(w, http.StatusConflict, "workspace already has a Pro subscription")
		return
	}
	session, err := billingProvider.Checkout(r.Context(), billing.CheckoutRequest{
		WorkspaceID: auth.WorkspaceID,
		Email:       auth.Email,
		CustomerID:  state.ProviderCustomerID,
		SuccessURL:  a.baseURL + "/dashboard?checkout=success",
		CancelURL:   a.baseURL + "/dashboard?checkout=canceled",
	})
	if errors.Is(err, billing.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	if err != nil {
		a.logger.Error("billing checkout creation failed", "workspace_id", auth.WorkspaceID, "error", err)
		writeError(w, http.StatusBadGateway, "billing checkout could not be created")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"checkout": session})
}

func (a *app) createBillingPortal(w http.ResponseWriter, r *http.Request) {
	auth, ok := a.billingAdmin(w, r)
	if !ok {
		return
	}
	state, err := a.store.GetBillingState(r.Context(), auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	if state.ProviderCustomerID == "" {
		writeError(w, http.StatusConflict, "workspace does not have a billing customer yet")
		return
	}
	session, err := billingProvider.Portal(r.Context(), state.ProviderCustomerID, a.baseURL+"/dashboard")
	if errors.Is(err, billing.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	if err != nil {
		a.logger.Error("billing portal creation failed", "workspace_id", auth.WorkspaceID, "error", err)
		writeError(w, http.StatusBadGateway, "billing portal could not be created")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"portal": session})
}

func (a *app) billingWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook body")
		return
	}
	event, err := billingProvider.VerifyWebhook(payload, r.Header.Get("Stripe-Signature"), time.Now().UTC())
	if errors.Is(err, billing.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook signature")
		return
	}
	claimed, err := a.store.ClaimBillingEvent(r.Context(), event.ID, time.Now().UTC())
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	if !claimed {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_processed"})
		return
	}
	processed := false
	defer func() {
		if !processed {
			if err := a.store.ReleaseBillingEvent(r.Context(), event.ID); err != nil {
				a.logger.Error("failed to release billing webhook claim", "event_id", event.ID, "error", err)
			}
		}
	}()
	if err := a.processBillingEvent(r, event); err != nil {
		a.logger.Error("billing webhook processing failed", "event_id", event.ID, "type", event.Type, "error", err)
		writeError(w, http.StatusInternalServerError, "webhook processing failed")
		return
	}
	processed = true
	writeJSON(w, http.StatusOK, map[string]string{"status": "processed"})
}

func (a *app) processBillingEvent(r *http.Request, event billing.Event) error {
	now := time.Now().UTC()
	switch event.Type {
	case "checkout.session.completed":
		var object struct {
			ClientReferenceID string            `json:"client_reference_id"`
			Customer          string            `json:"customer"`
			Subscription      string            `json:"subscription"`
			Metadata          map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(event.Data, &object); err != nil {
			return fmt.Errorf("decode checkout session: %w", err)
		}
		workspaceID := strings.TrimSpace(object.Metadata["workspace_id"])
		if workspaceID == "" {
			workspaceID = strings.TrimSpace(object.ClientReferenceID)
		}
		if workspaceID == "" {
			return nil
		}
		state := core.BillingState{
			WorkspaceID:            workspaceID,
			PlanCode:               core.PlanPro,
			Status:                 core.BillingStatusActive,
			ProviderCustomerID:     object.Customer,
			ProviderSubscriptionID: object.Subscription,
			UpdatedAt:              now,
		}
		return a.store.UpsertBillingState(r.Context(), state, billingAudit(workspaceID, "billing.checkout_completed", event.ID, now))

	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var object struct {
			ID                string            `json:"id"`
			Customer          string            `json:"customer"`
			Status            string            `json:"status"`
			CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
			CurrentPeriodEnd  int64             `json:"current_period_end"`
			Metadata          map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(event.Data, &object); err != nil {
			return fmt.Errorf("decode subscription: %w", err)
		}
		workspaceID := strings.TrimSpace(object.Metadata["workspace_id"])
		if workspaceID == "" {
			return nil
		}
		status := mapStripeSubscriptionStatus(object.Status)
		if event.Type == "customer.subscription.deleted" {
			status = core.BillingStatusCanceled
		}
		state := core.BillingState{
			WorkspaceID:            workspaceID,
			PlanCode:               core.PlanPro,
			Status:                 status,
			ProviderCustomerID:     object.Customer,
			ProviderSubscriptionID: object.ID,
			UpdatedAt:              now,
		}
		if object.CurrentPeriodEnd > 0 {
			periodEnd := time.Unix(object.CurrentPeriodEnd, 0).UTC()
			state.CurrentPeriodEnd = &periodEnd
		}
		metadata := map[string]string{"stripe_status": object.Status}
		if object.CancelAtPeriodEnd {
			metadata["cancel_at_period_end"] = "true"
		}
		audit := billingAudit(workspaceID, "billing.subscription_updated", event.ID, now)
		audit.Metadata = metadata
		return a.store.UpsertBillingState(r.Context(), state, audit)
	default:
		return nil
	}
}

func (a *app) billingAdmin(w http.ResponseWriter, r *http.Request) (core.AuthContext, bool) {
	auth, err := a.authorize(r, "workspace:admin", true)
	if err != nil {
		a.writeAuthError(w, err)
		return core.AuthContext{}, false
	}
	if auth.Credential != "session" || !isWorkspaceAdmin(auth) {
		writeError(w, http.StatusForbidden, "workspace owner or admin session required for billing")
		return core.AuthContext{}, false
	}
	return auth, true
}

func mapStripeSubscriptionStatus(status string) string {
	switch status {
	case "active", "trialing":
		return core.BillingStatusActive
	case "canceled", "incomplete_expired":
		return core.BillingStatusCanceled
	default:
		return core.BillingStatusPastDue
	}
}

func billingAudit(workspaceID, action, eventID string, now time.Time) core.AuditEntry {
	return core.AuditEntry{
		WorkspaceID:  workspaceID,
		Action:       action,
		ResourceType: "billing",
		ResourceID:   eventID,
		CreatedAt:    now,
	}
}

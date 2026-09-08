package handlers

import (
	"log"
	"net/http"
	"time"

	"construct/billing/internal/database"
	"construct/billing/internal/models"
	"construct/billing/internal/polar"
)

// ListPlans returns all active plans (public)
func ListPlans(w http.ResponseWriter, r *http.Request) {
	var plans []models.Plan
	database.DB.Where("is_active = ?", true).Order("sort_order ASC").Find(&plans)

	WriteJSON(w, 200, map[string]any{"plans": plans})
}

// GetSubscription returns the user's current subscription
func GetSubscription(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var sub models.Subscription
	result := database.DB.Preload("Plan").Where("user_id = ? AND status IN ?", userID, []string{"active", "trialing"}).First(&sub)
	if result.Error != nil {
		WriteJSON(w, 200, map[string]any{
			"subscription": nil,
			"plan":         map[string]any{"name": "Free", "slug": "free"},
		})
		return
	}

	WriteJSON(w, 200, map[string]any{
		"subscription": sub,
		"plan":         sub.Plan,
	})
}

// CancelSubscription cancels the user's current subscription
func CancelSubscription(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var sub models.Subscription
	result := database.DB.Where("user_id = ? AND status = ?", userID, "active").First(&sub)
	if result.Error != nil {
		WriteJSON(w, 404, map[string]any{"error": "no active subscription"})
		return
	}

	database.DB.Model(&sub).Updates(map[string]any{
		"cancel_at_period_end": true,
	})

	WriteJSON(w, 200, map[string]any{"status": "success", "message": "Subscription will cancel at end of period"})
}

// CreateCheckout creates a Polar checkout for a subscription plan.
func CreateCheckout(w http.ResponseWriter, r *http.Request) {
	body, err := parseBody(r)
	if err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid request body"})
		return
	}

	planSlug := getString(body, "plan_slug")
	if planSlug == "" {
		WriteJSON(w, 400, map[string]any{"error": "plan_slug is required"})
		return
	}

	var plan models.Plan
	if err := database.DB.Where("slug = ? AND is_active = ?", planSlug, true).First(&plan).Error; err != nil {
		WriteJSON(w, 404, map[string]any{"error": "plan not found"})
		return
	}
	if plan.PolarProductID == nil || *plan.PolarProductID == "" {
		WriteJSON(w, 422, map[string]any{"error": "plan is not configured for checkout"})
		return
	}

	userID := getUserID(r)
	successURL := getString(body, "success_url")
	returnURL := getString(body, "cancel_url")

	checkout, err := Polar.CreateCheckout(&polar.CheckoutRequest{
		Products:           []string{*plan.PolarProductID},
		ExternalCustomerID: userID,
		SuccessURL:         successURL,
		ReturnURL:          returnURL,
		Metadata: map[string]string{
			"type":      "subscription",
			"user_id":   userID,
			"plan_slug": plan.Slug,
		},
	})
	if err != nil {
		log.Printf("Polar checkout error for user %s plan %s: %v", userID, plan.Slug, err)
		WriteJSON(w, 502, map[string]any{"error": "failed to create checkout"})
		return
	}

	WriteJSON(w, 201, map[string]any{"url": checkout.URL, "checkout_id": checkout.ID})
}

// CreatePortalSession returns a Polar customer portal URL for the signed-in user.
func CreatePortalSession(w http.ResponseWriter, r *http.Request) {
	body, _ := parseBody(r)
	userID := getUserID(r)
	returnURL := getString(body, "return_url")

	session, err := Polar.CreateCustomerSession(userID, returnURL)
	if err != nil {
		log.Printf("Polar portal session error for user %s: %v", userID, err)
		WriteJSON(w, 502, map[string]any{"error": "failed to create billing portal"})
		return
	}
	WriteJSON(w, 201, map[string]any{"url": session.CustomerPortalURL, "expires_at": session.ExpiresAt})
}

// ListInvoices adapts local orders into the invoice shape expected by the app.
func ListInvoices(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var orders []models.Order
	database.DB.Where("user_id = ?", userID).Order("created_at DESC").Limit(50).Find(&orders)

	invoices := make([]map[string]any, 0, len(orders))
	for _, order := range orders {
		paidAt := any(nil)
		if order.Status == "paid" || order.Status == "succeeded" {
			paidAt = order.UpdatedAt.Format(time.RFC3339)
		}
		invoices = append(invoices, map[string]any{
			"id":                order.ID,
			"amount_due_cents":  order.Amount,
			"amount_paid_cents": order.Amount,
			"currency":          order.Currency,
			"status":            order.Status,
			"paid_at":           paidAt,
			"invoice_url":       "",
			"invoice_pdf":       "",
			"description":       order.ProductName,
			"created_at":        order.CreatedAt.Format(time.RFC3339),
		})
	}

	WriteJSON(w, 200, map[string]any{"invoices": invoices})
}

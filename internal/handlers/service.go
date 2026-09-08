package handlers

import (
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"

	"construct/billing/internal/database"
	"construct/billing/internal/models"
	"construct/billing/internal/polar"
)

// ServiceCreateCheckout creates a checkout for another service (e.g., domains)
func ServiceCreateCheckout(w http.ResponseWriter, r *http.Request) {
	body, err := parseBody(r)
	if err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}

	userID := getString(body, "user_id")

	source := getString(body, "source")
	successURL := getString(body, "success_url")

	itemsRaw, ok := body["items"].([]any)
	if !ok || len(itemsRaw) == 0 {
		WriteJSON(w, 400, map[string]any{"error": "items required"})
		return
	}

	var productIDs []string
	for _, raw := range itemsRaw {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := getString(item, "name")
		desc := getString(item, "description")
		price := 0
		if p, ok := item["price"].(float64); ok {
			price = int(p)
		}
		if name == "" || price < 50 {
			continue
		}

		product, err := Polar.CreateProduct(name, desc, price)
		if err != nil {
			log.Printf("Polar product create error: %v", err)
			continue
		}
		productIDs = append(productIDs, product.ID)
	}

	if len(productIDs) == 0 {
		WriteJSON(w, 400, map[string]any{"error": "no valid items"})
		return
	}

	checkout, err := Polar.CreateCheckout(&polar.CheckoutRequest{
		Products:   productIDs,
		SuccessURL: successURL,
		Metadata: map[string]string{
			"source":  source,
			"user_id": userID,
		},
	})
	if err != nil {
		log.Printf("Polar checkout error: %v", err)
		WriteJSON(w, 502, map[string]any{"error": "checkout creation failed"})
		return
	}

	WriteJSON(w, 201, map[string]any{
		"checkout_url": checkout.URL,
		"checkout_id":  checkout.ID,
	})
}

// ServiceGetBalance returns a user's credit balance
func ServiceGetBalance(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("userId")

	var balance models.CreditBalance
	if database.DB.Where("user_id = ?", uid).First(&balance).Error != nil {
		WriteJSON(w, 200, map[string]any{"balance": 0})
		return
	}
	WriteJSON(w, 200, map[string]any{"balance": balance.Balance})
}

// ServiceDeductCredits deducts credits from a user's balance
func ServiceDeductCredits(w http.ResponseWriter, r *http.Request) {
	body, err := parseBody(r)
	if err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}

	userID := getString(body, "user_id")
	amount := 0
	if a, ok := body["amount"].(float64); ok {
		amount = int(a)
	}
	description := getString(body, "description")
	reference := getString(body, "reference")

	if userID == "" || amount <= 0 {
		WriteJSON(w, 400, map[string]any{"error": "user_id and positive amount required"})
		return
	}

	// Atomic conditional update — previous read-check-write inside a plain
	// transaction let concurrent deductions both pass the balance check,
	// then overwrite each other. Conditional UPDATE lets the DB enforce
	// the "balance >= amount" invariant atomically; RowsAffected == 0
	// means either the row doesn't exist or the balance was insufficient
	// at the moment the row was locked.
	tx := database.DB.Begin()
	res := tx.Model(&models.CreditBalance{}).
		Where("user_id = ? AND balance >= ?", userID, amount).
		Update("balance", gorm.Expr("balance - ?", amount))
	if res.Error != nil {
		tx.Rollback()
		WriteJSON(w, 500, map[string]any{"error": "deduct failed: " + res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		tx.Rollback()
		// Re-read the balance so the response is honest about how much
		// the caller actually has. Non-existent row → balance 0.
		var current models.CreditBalance
		tx.Where("user_id = ?", userID).First(&current)
		WriteJSON(w, 400, map[string]any{"error": "insufficient credits", "balance": current.Balance, "required": amount})
		return
	}

	// Re-read to get the now-correct balance for the transaction row.
	var updated models.CreditBalance
	if err := tx.Where("user_id = ?", userID).First(&updated).Error; err != nil {
		tx.Rollback()
		WriteJSON(w, 500, map[string]any{"error": "balance refetch failed"})
		return
	}
	newBalance := updated.Balance

	transaction := models.CreditTransaction{
		UserID:       userID,
		Amount:       -amount,
		Type:         "usage",
		Description:  description,
		Reference:    reference,
		BalanceAfter: newBalance,
		CreatedAt:    time.Now(),
	}
	tx.Create(&transaction)
	tx.Commit()

	WriteJSON(w, 200, map[string]any{"status": "success", "balance": newBalance})
}

// ServiceGetSubscription returns a user's subscription status
func ServiceGetSubscription(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("userId")

	var sub models.Subscription
	if database.DB.Preload("Plan").Where("user_id = ? AND status IN ?", uid, []string{"active", "trialing"}).First(&sub).Error != nil {
		WriteJSON(w, 200, map[string]any{"plan": "free"})
		return
	}
	WriteJSON(w, 200, map[string]any{"plan": sub.Plan.Slug, "subscription": sub})
}

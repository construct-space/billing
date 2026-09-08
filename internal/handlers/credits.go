package handlers

import (
	"log"
	"net/http"

	"construct/billing/internal/database"
	"construct/billing/internal/models"
	"construct/billing/internal/polar"
)

// Credit product IDs on Polar
var CreditProducts = map[int]string{
	500:  "5e2674b6-8217-4d0e-bfcb-33932961691e",
	1000: "4831477a-bc8c-4f43-a7c6-d607263c9715",
	2500: "ad2011a2-53fc-4bd3-9818-3980983bd744",
	5000: "5878ce02-f766-4323-a401-14545708c89e",
}

// GetBalance returns the user's credit balance
func GetBalance(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var balance models.CreditBalance
	result := database.DB.Where("user_id = ?", userID).First(&balance)
	if result.Error != nil {
		WriteJSON(w, 200, map[string]any{"balance": 0, "currency": "EUR"})
		return
	}

	WriteJSON(w, 200, map[string]any{
		"balance":  balance.Balance,
		"currency": "EUR",
	})
}

// ListTransactions returns the user's credit transaction history
func ListTransactions(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var transactions []models.CreditTransaction
	database.DB.Where("user_id = ?", userID).Order("created_at DESC").Limit(50).Find(&transactions)

	WriteJSON(w, 200, map[string]any{
		"transactions": transactions,
	})
}

// PurchaseCredits creates a Polar checkout for credit purchase
func PurchaseCredits(w http.ResponseWriter, r *http.Request) {
	body, err := parseBody(r)
	if err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid request body"})
		return
	}

	productID := getString(body, "product_id")
	if productID == "" {
		WriteJSON(w, 400, map[string]any{"error": "product_id is required"})
		return
	}

	// Verify it's a known credit product
	valid := false
	for _, id := range Cfg.MediaCreditProducts {
		if id == productID {
			valid = true
			break
		}
	}
	if !valid {
		for _, id := range CreditProducts {
			if id == productID {
				valid = true
				break
			}
		}
	}
	if !valid {
		WriteJSON(w, 400, map[string]any{"error": "invalid product"})
		return
	}

	userID := getUserID(r)
	successURL := getString(body, "success_url")
	returnURL := getString(body, "cancel_url")
	if successURL == "" {
		successURL = Cfg.AppURL + "/credits?purchased=true"
	}

	checkout, err := Polar.CreateCheckout(&polar.CheckoutRequest{
		Products:           []string{productID},
		ExternalCustomerID: userID,
		SuccessURL:         successURL,
		ReturnURL:          returnURL,
		Metadata: map[string]string{
			"type":    "credits",
			"user_id": userID,
		},
	})
	if err != nil {
		log.Printf("Polar checkout error for user %s: %v", userID, err)
		WriteJSON(w, 502, map[string]any{"error": "failed to create checkout"})
		return
	}

	WriteJSON(w, 201, map[string]any{
		"url":          checkout.URL,
		"checkout_url": checkout.URL,
		"checkout_id":  checkout.ID,
	})
}

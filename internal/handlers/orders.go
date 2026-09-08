package handlers

import (
	"net/http"

	"construct/billing/internal/database"
	"construct/billing/internal/models"
)

// ListOrders returns the user's order history
func ListOrders(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var orders []models.Order
	database.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&orders)

	WriteJSON(w, 200, map[string]any{
		"orders": orders,
	})
}

// GetOrder returns a single order
func GetOrder(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	id := r.PathValue("id")

	var order models.Order
	if err := database.DB.Where("id = ? AND user_id = ?", id, userID).First(&order).Error; err != nil {
		WriteJSON(w, 404, map[string]any{"error": "order not found"})
		return
	}

	WriteJSON(w, 200, map[string]any{"order": order})
}

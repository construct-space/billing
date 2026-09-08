package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"construct/billing/internal/database"
	"construct/billing/internal/models"
)

// Maximum age for an incoming webhook, measured against webhook-timestamp.
// Polar sends webhook-timestamp in unix seconds; anything older than this
// is treated as a replay and rejected. 5 minutes matches Standard Webhooks'
// default tolerance and is well beyond normal network latency.
const webhookMaxAge = 5 * time.Minute

// PolarWebhook handles incoming Polar webhook events.
//
// Polar uses the Standard Webhooks spec (https://www.standardwebhooks.com):
//   - webhook-id:        unique message id
//   - webhook-timestamp: unix seconds when the event was signed
//   - webhook-signature: "v1,<base64(hmac_sha256)>" — space-separated if
//                        multiple signatures are present (for key rotation)
//
// The HMAC is computed over "{webhook-id}.{webhook-timestamp}.{body}" with
// the secret. Polar secrets are base64-encoded and prefixed with whsec_;
// we strip the prefix and base64-decode before using as the HMAC key.
//
// Previously this handler trusted the body unconditionally, which meant
// any POST to /api/webhooks/polar could fabricate an order.created event
// and credit any user_id — a P0 from code review.
func PolarWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteJSON(w, 400, map[string]any{"error": "failed to read body"})
		return
	}
	defer r.Body.Close()

	if err := verifyPolarSignature(r, body); err != nil {
		log.Printf("[webhook] signature verification failed: %v", err)
		WriteJSON(w, 401, map[string]any{"error": "invalid signature"})
		return
	}

	var event struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid JSON"})
		return
	}

	log.Printf("[webhook] Event: %s", event.Type)

	switch event.Type {
	case "order.created":
		handleOrderCreated(event.Data)
	case "subscription.created", "subscription.updated":
		handleSubscriptionUpdated(event.Data)
	case "subscription.canceled":
		handleSubscriptionCanceled(event.Data)
	}

	WriteJSON(w, 200, map[string]any{"received": true})
}

// verifyPolarSignature enforces the Standard Webhooks algorithm. Returns
// nil on success; any non-nil error means the request should be rejected
// with 401.
//
// Fails closed: if POLAR_WEBHOOK_SECRET isn't set and APP_ENV != dev, we
// refuse to accept webhooks entirely. Reading the secret via the global
// Cfg pointer — set at startup.
func verifyPolarSignature(r *http.Request, body []byte) error {
	secret := webhookSecret()
	if secret == "" {
		if isDev() {
			log.Printf("[webhook] WARNING: POLAR_WEBHOOK_SECRET is unset in dev — skipping verification")
			return nil
		}
		return fmt.Errorf("POLAR_WEBHOOK_SECRET not configured")
	}

	msgID := r.Header.Get("webhook-id")
	msgTS := r.Header.Get("webhook-timestamp")
	msgSig := r.Header.Get("webhook-signature")
	if msgID == "" || msgTS == "" || msgSig == "" {
		return fmt.Errorf("missing Standard Webhooks headers")
	}

	// Replay protection — reject stale timestamps.
	ts, err := strconv.ParseInt(msgTS, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	age := time.Since(time.Unix(ts, 0))
	if age < -webhookMaxAge || age > webhookMaxAge {
		return fmt.Errorf("timestamp outside tolerance (%s)", age)
	}

	// Standard Webhooks content to HMAC: "{id}.{ts}.{body}"
	toSign := []byte(msgID + "." + msgTS + ".")
	toSign = append(toSign, body...)

	key, err := decodeSecret(secret)
	if err != nil {
		return fmt.Errorf("decode secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(toSign)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Header may carry multiple signatures for key rotation:
	// "v1,<sig1> v1,<sig2>". Any matching signature wins. Constant-time
	// compare prevents timing leaks.
	for _, part := range strings.Fields(msgSig) {
		version, sig, ok := strings.Cut(part, ",")
		if !ok || version != "v1" {
			continue
		}
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}
	return fmt.Errorf("no valid signature")
}

// webhookSecret reads POLAR_WEBHOOK_SECRET via the global Cfg struct so
// tests can swap it in. Falls back to the env var if Cfg isn't wired.
func webhookSecret() string {
	if Cfg != nil {
		return Cfg.PolarWebhookSecret
	}
	return os.Getenv("POLAR_WEBHOOK_SECRET")
}

// isDev is a lenient check for local development — we tolerate a missing
// secret only when APP_ENV explicitly says dev, so prod (unset var)
// still fails closed.
func isDev() bool {
	env := strings.ToLower(os.Getenv("APP_ENV"))
	return env == "dev" || env == "development" || env == "local"
}

// decodeSecret strips the whsec_ prefix and base64-decodes the remainder.
// Polar secrets are issued as whsec_<base64>. If the prefix is absent we
// treat the whole thing as a raw key (older or custom setups).
func decodeSecret(secret string) ([]byte, error) {
	s := strings.TrimPrefix(secret, "whsec_")
	// Try standard base64 first, then raw — be permissive about padding.
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	// Fall back to treating the whole string as the key.
	return []byte(secret), nil
}

func handleOrderCreated(data map[string]any) {
	orderID := fmt.Sprintf("%v", data["id"])
	amount := 0
	if a, ok := data["amount"].(float64); ok {
		amount = int(a)
	}

	productID := ""
	if p, ok := data["product_id"].(string); ok {
		productID = p
	}

	// Check if already processed (idempotency)
	var existing models.Order
	if database.DB.Where("polar_order_id = ?", orderID).First(&existing).Error == nil {
		log.Printf("[webhook] Order %s already processed", orderID)
		return
	}

	// Determine product type
	productType := "unknown"
	productName := ""
	for cents, pid := range CreditProducts {
		if pid == productID {
			productType = "credits"
			productName = fmt.Sprintf("Credits €%d", cents/100)
			break
		}
	}

	// Get user ID (UUID string) from metadata
	userID := ""
	if meta, ok := data["metadata"].(map[string]any); ok {
		if uid, ok := meta["user_id"].(string); ok {
			userID = uid
		}
	}

	// Create order record
	order := models.Order{
		PolarOrderID:   orderID,
		PolarProductID: productID,
		UserID:         userID,
		ProductType:    productType,
		ProductName:    productName,
		Amount:         amount,
		Currency:       "EUR",
		Status:         "completed",
	}
	database.DB.Create(&order)

	// Credit balance if it's a credit purchase
	if productType == "credits" && userID != "" {
		creditUser(userID, amount, "purchase", productName, &order.ID)
	}

	log.Printf("[webhook] Order %s processed: %s %d cents for user %s", orderID, productType, amount, userID)
}

func creditUser(userID string, amount int, txType, description string, orderID *uint) {
	tx := database.DB.Begin()

	var balance models.CreditBalance
	result := tx.Where("user_id = ?", userID).First(&balance)
	if result.Error != nil {
		balance = models.CreditBalance{UserID: userID, Balance: 0}
		tx.Create(&balance)
	}

	newBalance := balance.Balance + amount
	tx.Model(&balance).Update("balance", newBalance)

	transaction := models.CreditTransaction{
		UserID:       userID,
		Amount:       amount,
		Type:         txType,
		Description:  description,
		OrderID:      orderID,
		BalanceAfter: newBalance,
		CreatedAt:    time.Now(),
	}
	tx.Create(&transaction)

	tx.Commit()
}

func handleSubscriptionUpdated(data map[string]any) {
	log.Printf("[webhook] Subscription updated: %v", data["id"])
}

func handleSubscriptionCanceled(data map[string]any) {
	log.Printf("[webhook] Subscription canceled: %v", data["id"])
}

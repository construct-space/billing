package main

import (
	"log"
	"net/http"
	"time"

	"construct/billing/internal/config"
	"construct/billing/internal/database"
	"construct/billing/internal/handlers"
	"construct/billing/internal/middleware"
	"construct/billing/internal/polar"
)

func main() {
	cfg := config.Load()
	handlers.Cfg = cfg
	handlers.Polar = polar.New(cfg.PolarToken)

	database.Init(cfg)

	mux := http.NewServeMux()

	auth := middleware.Auth(cfg)
	svc := middleware.ServiceAuth(cfg)

	// Public
	mux.HandleFunc("GET /api/plans", handlers.ListPlans)

	// OAuth
	mux.HandleFunc("GET /api/auth/login", handlers.LoginRedirect)
	mux.HandleFunc("GET /api/auth/register", handlers.RegisterRedirect)
	mux.HandleFunc("GET /api/auth/callback", handlers.AuthCallback)
	mux.HandleFunc("GET /api/auth/me", handlers.AuthMe)
	mux.HandleFunc("GET /api/auth/logout", handlers.Logout)

	// Webhooks (Polar)
	mux.HandleFunc("POST /api/webhooks/polar", handlers.PolarWebhook)

	// Credits (authenticated)
	mux.Handle("GET /api/credits", auth(http.HandlerFunc(handlers.GetBalance)))
	mux.Handle("GET /api/credits/transactions", auth(http.HandlerFunc(handlers.ListTransactions)))
	mux.Handle("POST /api/credits/purchase", auth(http.HandlerFunc(handlers.PurchaseCredits)))

	// Subscriptions (authenticated)
	mux.Handle("GET /api/subscription", auth(http.HandlerFunc(handlers.GetSubscription)))
	mux.Handle("POST /api/subscription/cancel", auth(http.HandlerFunc(handlers.CancelSubscription)))
	mux.Handle("DELETE /api/subscription", auth(http.HandlerFunc(handlers.CancelSubscription)))
	mux.Handle("POST /api/checkout", auth(http.HandlerFunc(handlers.CreateCheckout)))
	mux.Handle("POST /api/portal", auth(http.HandlerFunc(handlers.CreatePortalSession)))
	mux.Handle("GET /api/invoices", auth(http.HandlerFunc(handlers.ListInvoices)))

	// Orders (authenticated)
	mux.Handle("GET /api/orders", auth(http.HandlerFunc(handlers.ListOrders)))
	mux.Handle("GET /api/orders/{id}", auth(http.HandlerFunc(handlers.GetOrder)))

	// Service-to-service API
	mux.Handle("POST /api/service/checkout", svc(http.HandlerFunc(handlers.ServiceCreateCheckout)))
	mux.Handle("GET /api/service/balance/{userId}", svc(http.HandlerFunc(handlers.ServiceGetBalance)))
	mux.Handle("POST /api/service/deduct", svc(http.HandlerFunc(handlers.ServiceDeductCredits)))
	mux.Handle("GET /api/service/subscription/{userId}", svc(http.HandlerFunc(handlers.ServiceGetSubscription)))

	// Health
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.WriteJSON(w, 200, map[string]any{"status": "ok"})
	})

	// Root — minimal JSON identity. Any non-/api path 404s; the customer UI
	// is served by the portal gateway.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		handlers.WriteJSON(w, 200, map[string]any{"service": "billing-api", "status": "ok"})
	})

	// Middleware
	var handler http.Handler = mux
	handler = middleware.CORS(cfg)(handler)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.Logger(handler)

	log.Printf("Construct Billing running on :%s", cfg.Port)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

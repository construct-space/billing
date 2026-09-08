package middleware

import (
	"crypto/subtle"
	"net/http"

	"construct/billing/internal/config"
	"construct/billing/internal/database"
	"construct/billing/internal/models"
)

func Auth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.InternalSharedSecret != "" && r.Header.Get("X-Internal-Secret") == cfg.InternalSharedSecret {
				if userID := r.Header.Get("X-Auth-User-ID"); userID != "" {
					r.Header.Set("X-User-ID", userID)
					next.ServeHTTP(w, r)
					return
				}
			}
			session := GetSession(r)
			if session == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			r.Header.Set("X-User-ID", session.UserID)
			next.ServeHTTP(w, r)
		})
	}
}

func GetSession(r *http.Request) *models.Session {
	token := ParseCookie(r.Header.Get("Cookie"), "session")
	if token == "" {
		return nil
	}
	var session models.Session
	if err := database.DB.Where("token = ?", token).First(&session).Error; err != nil {
		return nil
	}
	if session.IsExpired() {
		return nil
	}
	return &session
}

// ServiceAuth validates inter-service API key.
//
// Fails closed when SERVICE_API_KEY is unset — otherwise a misconfigured
// deploy would accept "Authorization: Bearer " (the literal string with
// no token) because "Bearer "+"" matches and opens every /api/service/*
// endpoint to anonymous callers. Previous implementation relied on the
// header being non-empty, which "Bearer " satisfies.
func ServiceAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.ServiceAPIKey == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"service auth not configured"}`))
				return
			}
			expected := "Bearer " + cfg.ServiceAPIKey
			got := r.Header.Get("Authorization")
			if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid service key"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

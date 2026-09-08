package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"construct/billing/internal/database"
	"construct/billing/internal/middleware"
	"construct/billing/internal/models"
)

var (
	pendingStatesMu sync.Mutex
	pendingStates   = make(map[string]time.Time)
)

func generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSessionToken() string {
	b := make([]byte, 48)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func LoginRedirect(w http.ResponseWriter, r *http.Request) {
	state := generateState()
	pendingStatesMu.Lock()
	pendingStates[state] = time.Now().Add(10 * time.Minute)
	now := time.Now()
	for k, exp := range pendingStates {
		if now.After(exp) {
			delete(pendingStates, k)
		}
	}
	pendingStatesMu.Unlock()

	params := url.Values{
		"client_id":     {Cfg.OAuthClientID},
		"redirect_uri":  {Cfg.OAuthRedirectURI},
		"response_type": {"code"},
		"scope":         {"profile email"},
		"state":         {state},
	}
	w.Header().Set("Location", Cfg.OAuthURL+"/oauth/authorize?"+params.Encode())
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

func RegisterRedirect(w http.ResponseWriter, r *http.Request) {
	state := generateState()
	pendingStatesMu.Lock()
	pendingStates[state] = time.Now().Add(10 * time.Minute)
	now := time.Now()
	for k, exp := range pendingStates {
		if now.After(exp) {
			delete(pendingStates, k)
		}
	}
	pendingStatesMu.Unlock()

	params := url.Values{
		"client_id":     {Cfg.OAuthClientID},
		"redirect_uri":  {Cfg.OAuthRedirectURI},
		"response_type": {"code"},
		"scope":         {"profile email"},
		"state":         {state},
	}
	w.Header().Set("Location", Cfg.OAuthURL+"/register?"+params.Encode())
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

func AuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")

	if errParam != "" {
		redirectWithError(w, "OAuth error: "+errParam)
		return
	}
	if code == "" {
		redirectWithError(w, "Missing authorization code")
		return
	}

	pendingStatesMu.Lock()
	expiry, exists := pendingStates[state]
	if exists {
		delete(pendingStates, state)
	}
	pendingStatesMu.Unlock()

	if !exists || time.Now().After(expiry) {
		redirectWithError(w, "Invalid or expired state")
		return
	}

	tokenData, err := exchangeCode(code)
	if err != nil {
		log.Printf("[auth] Token exchange error: %v", err)
		redirectWithError(w, "Failed to exchange authorization code")
		return
	}

	accessToken, ok := tokenData["access_token"].(string)
	if !ok || accessToken == "" {
		redirectWithError(w, "No access token received")
		return
	}

	userInfo, err := fetchUserInfo(accessToken)
	if err != nil {
		log.Printf("[auth] User info error: %v", err)
		redirectWithError(w, "Failed to fetch user profile")
		return
	}

	userID, _ := userInfo["id"].(string)
	if userID == "" {
		redirectWithError(w, "Invalid user ID")
		return
	}

	sessionToken := generateSessionToken()
	ua := r.Header.Get("User-Agent")
	ip := getClientIP(r)
	session := models.Session{
		Token: sessionToken, UserID: userID,
		UserAgent: &ua, IPAddress: &ip,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}
	database.DB.Create(&session)
	database.DB.Where("expires_at < ?", time.Now()).Delete(&models.Session{})

	w.Header().Set("Location", Cfg.AppURL+"/dashboard")
	w.Header().Set("Set-Cookie", middleware.SessionCookie(Cfg, sessionToken, false))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

func AuthMe(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		WriteJSON(w, 200, map[string]any{"authenticated": false})
		return
	}
	WriteJSON(w, 200, map[string]any{"authenticated": true, "user": map[string]any{"id": session.UserID}})
}

func Logout(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session != nil {
		database.DB.Delete(session)
	}
	w.Header().Set("Location", Cfg.AppURL)
	w.Header().Set("Set-Cookie", middleware.SessionCookie(Cfg, "", true))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

func redirectWithError(w http.ResponseWriter, message string) {
	w.Header().Set("Location", "/?error="+url.QueryEscape(message))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

func exchangeCode(code string) (map[string]any, error) {
	body := map[string]string{
		"grant_type": "authorization_code", "code": code,
		"client_id": Cfg.OAuthClientID, "client_secret": Cfg.OAuthClientSecret,
		"redirect_uri": Cfg.OAuthRedirectURI,
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", Cfg.OAuthURL+"/oauth/token", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange failed: %d", resp.StatusCode)
	}
	return result, nil
}

func fetchUserInfo(accessToken string) (map[string]any, error) {
	req, _ := http.NewRequest("GET", Cfg.OAuthURL+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("user info failed: %d", resp.StatusCode)
	}
	return result, nil
}

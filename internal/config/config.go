package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                 string
	AppURL               string
	AllowedOrigins       []string
	DBDriver             string
	DBHost               string
	DBPort               string
	DBName               string
	DBUser               string
	DBPass               string
	OAuthURL             string
	OAuthClientID        string
	OAuthClientSecret    string
	OAuthRedirectURI     string
	PolarToken           string
	PolarWebhookSecret   string
	InternalSharedSecret string
	ServiceAPIKey        string
	MediaCreditProducts  []string
}

func Load() *Config {
	loadEnvFile(".env")

	origins := strings.Split(env("ALLOWED_ORIGINS", "http://localhost:8002,http://localhost:3051,tauri://localhost"), ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	return &Config{
		Port:                 env("PORT", "8002"),
		AppURL:               env("APP_URL", "http://localhost:8002"),
		AllowedOrigins:       origins,
		DBDriver:             env("DB_DRIVER", "mysql"),
		DBHost:               env("DB_HOST", "localhost"),
		DBPort:               env("DB_PORT", "3306"),
		DBName:               env("DB_NAME", "construct_billing"),
		DBUser:               env("DB_USER", "root"),
		DBPass:               env("DB_PASS", ""),
		OAuthURL:             env("OAUTH_URL", "https://accounts.lisaos.dev"),
		OAuthClientID:        env("OAUTH_CLIENT_ID", ""),
		OAuthClientSecret:    env("OAUTH_CLIENT_SECRET", ""),
		OAuthRedirectURI:     env("OAUTH_REDIRECT_URI", "http://localhost:8002/api/auth/callback"),
		PolarToken:           env("POLAR_TOKEN", ""),
		PolarWebhookSecret:   env("POLAR_WEBHOOK_SECRET", ""),
		InternalSharedSecret: env("INTERNAL_SHARED_SECRET", ""),
		ServiceAPIKey:        env("SERVICE_API_KEY", ""),
		MediaCreditProducts:  splitCSV(env("MEDIA_CREDIT_PRODUCT_IDS", "5e2674b6-8217-4d0e-bfcb-33932961691e,4831477a-bc8c-4f43-a7c6-d607263c9715,ad2011a2-53fc-4bd3-9818-3980983bd744,5878ce02-f766-4323-a401-14545708c89e")),
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (c *Config) IsSecure() bool {
	return strings.HasPrefix(c.AppURL, "https")
}

func SessionCookie(cfg *Config, token string, clear bool) string {
	maxAge := 30 * 24 * 60 * 60
	value := token
	if clear {
		maxAge = 0
		value = ""
	}
	cookie := fmt.Sprintf("session=%s; Path=/; HttpOnly; SameSite=Lax; Max-Age=%d", value, maxAge)
	if cfg.IsSecure() {
		cookie += "; Secure"
	}
	return cookie
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			key := strings.TrimSpace(k)
			val := strings.TrimSpace(v)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.LogLevel != slog.LevelInfo || cfg.LogFormat != "text" ||
		cfg.ShutdownTimeout != 10*time.Second || cfg.SessionTTL != 24*time.Hour ||
		!cfg.CookieSecure || cfg.DBMaxConns != 0 || cfg.DocumentStorageDir != "storage/documents" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"DATABASE_URL":         "postgres://x",
		"HTTP_ADDR":            ":9090",
		"LOG_LEVEL":            "debug",
		"LOG_FORMAT":           "json",
		"SHUTDOWN_TIMEOUT":     "3s",
		"SESSION_TTL":          "12h",
		"COOKIE_SECURE":        "false",
		"DB_MAX_CONNS":         "8",
		"DOCUMENT_STORAGE_DIR": "/var/lib/lp/documents",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.LogLevel != slog.LevelDebug || cfg.LogFormat != "json" ||
		cfg.ShutdownTimeout != 3*time.Second || cfg.SessionTTL != 12*time.Hour ||
		cfg.CookieSecure || cfg.DBMaxConns != 8 || cfg.DocumentStorageDir != "/var/lib/lp/documents" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidSessionSettings(t *testing.T) {
	for _, settings := range []map[string]string{
		{"SESSION_TTL": "invalid"},
		{"SESSION_TTL": "0s"},
		{"SESSION_TTL": "-1h"},
		{"COOKIE_SECURE": "sometimes"},
	} {
		values := map[string]string{"DATABASE_URL": "postgres://x"}
		for key, value := range settings {
			values[key] = value
		}
		if _, err := load(env(values)); err == nil {
			t.Errorf("load(%v) returned no error", settings)
		}
	}
}

func TestLoadCollectsAllErrors(t *testing.T) {
	_, err := load(env(map[string]string{
		"LOG_LEVEL":        "loud",
		"LOG_FORMAT":       "xml",
		"SHUTDOWN_TIMEOUT": "soon",
		"SESSION_TTL":      "soon",
		"COOKIE_SECURE":    "maybe",
		"DB_MAX_CONNS":     "-1",
	}))
	if err == nil {
		t.Fatal("want error")
	}
	for _, key := range []string{"DATABASE_URL", "LOG_LEVEL", "LOG_FORMAT", "SHUTDOWN_TIMEOUT", "SESSION_TTL", "COOKIE_SECURE", "DB_MAX_CONNS"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}

func TestLoadCORSAllowedOrigins(t *testing.T) {
	cfg, err := load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Fatalf("CORS enabled by default: %v", cfg.CORSAllowedOrigins)
	}

	cfg, err = load(env(map[string]string{
		"DATABASE_URL":         "postgres://x",
		"CORS_ALLOWED_ORIGINS": "http://localhost:5173, https://app.example.com",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{"http://localhost:5173", "https://app.example.com"}
	if strings.Join(cfg.CORSAllowedOrigins, "|") != strings.Join(want, "|") {
		t.Fatalf("CORSAllowedOrigins = %v, want %v", cfg.CORSAllowedOrigins, want)
	}
}

// Origin в браузере — только scheme://host[:port]; значение с путём или без схемы
// никогда не совпадёт, и CORS молча не заработает — лучше не стартовать.
func TestLoadRejectsInvalidCORSOrigins(t *testing.T) {
	for _, origins := range []string{
		"localhost:5173",
		"http://localhost:5173/",
		"http://localhost:5173/app",
		"https://app.example.com?x=1",
		"*",
		"ftp://files.example.com",
	} {
		_, err := load(env(map[string]string{"DATABASE_URL": "postgres://x", "CORS_ALLOWED_ORIGINS": origins}))
		if err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
			t.Errorf("load(CORS_ALLOWED_ORIGINS=%q) error = %v, want CORS_ALLOWED_ORIGINS error", origins, err)
		}
	}
}

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
		cfg.ShutdownTimeout != 10*time.Second || cfg.DBMaxConns != 0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"DATABASE_URL":     "postgres://x",
		"HTTP_ADDR":        ":9090",
		"LOG_LEVEL":        "debug",
		"LOG_FORMAT":       "json",
		"SHUTDOWN_TIMEOUT": "3s",
		"DB_MAX_CONNS":     "8",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.LogLevel != slog.LevelDebug || cfg.LogFormat != "json" ||
		cfg.ShutdownTimeout != 3*time.Second || cfg.DBMaxConns != 8 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadCollectsAllErrors(t *testing.T) {
	_, err := load(env(map[string]string{
		"LOG_LEVEL":        "loud",
		"LOG_FORMAT":       "xml",
		"SHUTDOWN_TIMEOUT": "soon",
		"DB_MAX_CONNS":     "-1",
	}))
	if err == nil {
		t.Fatal("want error")
	}
	for _, key := range []string{"DATABASE_URL", "LOG_LEVEL", "LOG_FORMAT", "SHUTDOWN_TIMEOUT", "DB_MAX_CONNS"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}

// Package config читает настройки сервера из переменных окружения.
//
// Локально переменные приходят из .env через Makefile (-include .env + export),
// в контейнере — из окружения. Список и дефолты — в .env.example.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// DatabaseURL — строка подключения pgx (обязательна).
	DatabaseURL string
	// DBMaxConns — размер пула; 0 = дефолт pgxpool (max(4, NumCPU)).
	DBMaxConns int32
	// HTTPAddr — адрес, который слушает сервер, например ":8080".
	HTTPAddr string
	// LogLevel — debug | info | warn | error.
	LogLevel slog.Level
	// LogFormat — text (локально) | json (в контейнере, для сборщика логов).
	LogFormat string
	// ShutdownTimeout — сколько ждать завершения активных запросов при остановке.
	ShutdownTimeout time.Duration
	// SessionTTL — срок жизни сессии.
	SessionTTL time.Duration
	// CookieSecure включает флаг Secure у cookie.
	CookieSecure bool
}

// Load читает конфиг из окружения. Возвращает все ошибки разом, а не первую.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := Config{
		DatabaseURL: get("DATABASE_URL", ""),
		HTTPAddr:    get("HTTP_ADDR", ":8080"),
		LogFormat:   get("LOG_FORMAT", "text"),
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		errs = append(errs, fmt.Errorf("LOG_FORMAT: want text or json, got %q", cfg.LogFormat))
	}

	sessionTTL, err := time.ParseDuration(get("SESSION_TTL", "24h"))
	if err != nil || sessionTTL <= 0 {
		errs = append(errs, fmt.Errorf("SESSION_TTL: want positive duration like 24h, got %q", get("SESSION_TTL", "")))
	}
	cfg.SessionTTL = sessionTTL

	cookieSecure, err := strconv.ParseBool(get("COOKIE_SECURE", "true"))
	if err != nil {
		errs = append(errs, fmt.Errorf("COOKIE_SECURE: want boolean, got %q", get("COOKIE_SECURE", "")))
	}
	cfg.CookieSecure = cookieSecure

	timeout, err := time.ParseDuration(get("SHUTDOWN_TIMEOUT", "10s"))
	if err != nil || timeout <= 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: want positive duration like 10s, got %q", get("SHUTDOWN_TIMEOUT", "")))
	}
	cfg.ShutdownTimeout = timeout

	maxConns, err := strconv.ParseInt(get("DB_MAX_CONNS", "0"), 10, 32)
	if err != nil || maxConns < 0 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS: want non-negative integer, got %q", get("DB_MAX_CONNS", "")))
	}
	cfg.DBMaxConns = int32(maxConns)

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return cfg, nil
}

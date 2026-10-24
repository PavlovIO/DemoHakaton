// Package config читает настройки сервера из переменных окружения.
//
// Локально переменные приходят из .env через Makefile (-include .env + export),
// в контейнере — из окружения. Список и дефолты — в .env.example.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
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
	// DocumentStorageDir — каталог файлов KYC-документов вне веб-корня; должен переживать
	// перезапуски (в контейнере — volume).
	DocumentStorageDir string
	// CORSAllowedOrigins — origin'ы фронта на другом хосте (scheme://host[:port]).
	// Пусто — CORS выключен: основной путь — фронт и API на одном origin.
	CORSAllowedOrigins []string
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

		DocumentStorageDir: get("DOCUMENT_STORAGE_DIR", "storage/documents"),
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

	origins, err := parseOrigins(get("CORS_ALLOWED_ORIGINS", ""))
	if err != nil {
		errs = append(errs, fmt.Errorf("CORS_ALLOWED_ORIGINS: %w", err))
	}
	cfg.CORSAllowedOrigins = origins

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return cfg, nil
}

// parseOrigins разбирает список origin'ов через запятую. Браузер шлёт Origin ровно как
// scheme://host[:port], сравнение — точное, поэтому путь, query или «*» — ошибка конфига:
// такое значение никогда не совпадёт или (для «*») открыло бы API любому сайту.
func parseOrigins(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var origins []string
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.Path != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(origin, "#") || u.User != nil {
			return nil, fmt.Errorf("want scheme://host[:port], got %q", origin)
		}
		// Браузер сериализует Origin в нижнем регистре и без порта по умолчанию:
		// «https://App.example.com:443» иначе никогда не совпало бы.
		host := strings.ToLower(u.Host)
		if port := u.Port(); (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
			host = strings.TrimSuffix(host, ":"+port)
		}
		origins = append(origins, u.Scheme+"://"+host)
	}
	return origins, nil
}

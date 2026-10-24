// Команда server — HTTP API платформы ликвидности.
//
// Запуск локально: make run (переменные окружения берутся из .env).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/config"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/storage"
)

const dbConnectTimeout = 5 * time.Second

func main() {
	// Ctrl+C / SIGTERM (docker stop) → контекст отменяется → graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("server failed", "err", err)
		stop()
		os.Exit(1)
	}
}

// run работает до отмены ctx, затем корректно останавливает сервер и пул.
func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(newLogger(cfg))

	pool, err := newPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Каталог документов недоступен для записи — сервер не стартует, а не падает на загрузке.
	documents, err := storage.NewLocal(cfg.DocumentStorageDir)
	if err != nil {
		return err
	}

	queries := repository.New(pool)
	txManager := service.NewTxManager(pool)
	auth := service.NewAuth(queries, txManager, cfg.SessionTTL, service.NewMemoryRateLimiter(), service.DefaultAuthLimits)
	h := handler.New(handler.Deps{
		DB:    pool,
		Dicts: service.NewDictionaries(queries),
		Auth:  auth,
		Kyc: service.NewKyc(service.KycDeps{
			Store: queries, Tx: txManager, Files: documents,
			Screener: aml.NoopScreener{}, // правила подключает BE-13: aml.NewRuleScreener
			Verifier: nil,                // сверку с реестром подключает BE-06: registry.New(cfg)
		}),
		CookieSecure: cfg.CookieSecure,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.NewRouter(h),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("http server started", "addr", cfg.HTTPAddr, "base_url", handler.BaseURL)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		// Сервер не поднялся (например, порт занят).
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	slog.Info("shutting down", "timeout", cfg.ShutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}
	slog.Info("stopped")
	return nil
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// newPool создаёт пул и сразу проверяет соединение: без БД сервер не стартует (fail fast).
func newPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if cfg.DBMaxConns > 0 {
		poolCfg.MaxConns = cfg.DBMaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create db pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, dbConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	slog.Info("database connected", "max_conns", poolCfg.MaxConns)
	return pool, nil
}

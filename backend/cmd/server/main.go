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
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/dictionaries"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/notifications"
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
	// Один ограничитель на процесс: ключи login/register/registry_lookup/chat:send различаются префиксом.
	limiter := service.NewMemoryRateLimiter()
	authService := auth.NewAuth(queries, txManager, cfg.SessionTTL, limiter, auth.DefaultAuthLimits)
	companyRegistry := registry.NewMock() // в MVP реестр — только мок (DECISIONS п. 17)
	// AML-скрининг читает только локальную БД (санкции, FATF): submitKyc и submit заявки
	// при его ошибке откатываются (fail-closed).
	screener := aml.RuleScreener{}
	h := handler.New(handler.Deps{
		DB:    pool,
		Dicts: dictionaries.NewDictionaries(queries),
		Auth:  authService,
		Kyc: kyc.NewKyc(kyc.KycDeps{
			Store: queries, Tx: txManager, Files: documents,
			Screener: screener,
			Verifier: companyRegistry,
		}),
		Registry: kyc.NewRegistry(companyRegistry, limiter),
		Applications: lending.NewApplications(lending.ApplicationsDeps{
			Store: queries, Tx: txManager,
			Screener: screener,
		}),
		Offers:        lending.NewOffers(lending.OffersDeps{Store: queries, Tx: txManager}),
		Deals:         lending.NewDeals(lending.DealsDeps{Store: queries, Tx: txManager}),
		Chat:          lending.NewChat(lending.ChatDeps{Store: queries, Tx: txManager, Limiter: limiter}),
		Notifications: notifications.NewNotifications(queries, txManager),
		Admin:         admin.New(admin.Deps{Tx: txManager}),
		CookieSecure:  cfg.CookieSecure,
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

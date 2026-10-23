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
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/config"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
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

const (
	dbConnectTimeout = 5 * time.Second
	// dbApplicationName — имя соединений сервера в pg_stat_activity.
	dbApplicationName = "lp-backend"
)

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

	router, err := newRouter(pool, cfg)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
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

// newRouter собирает граф зависимостей API поверх пула: его же поднимает TestIDOR,
// чтобы проверять доступ на тех же сервисах и SQL, что в проде.
func newRouter(pool *pgxpool.Pool, cfg config.Config) (http.Handler, error) {
	// Каталог документов недоступен для записи — сервер не стартует, а не падает на загрузке.
	documents, err := storage.NewLocal(cfg.DocumentStorageDir)
	if err != nil {
		return nil, err
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
	return handler.NewRouter(handler.New(handler.Deps{
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
		Offers:             lending.NewOffers(lending.OffersDeps{Store: queries, Tx: txManager}),
		Deals:              lending.NewDeals(lending.DealsDeps{Store: queries, Tx: txManager}),
		Chat:               lending.NewChat(lending.ChatDeps{Store: queries, Tx: txManager, Limiter: limiter}),
		Notifications:      notifications.NewNotifications(queries, txManager),
		Admin:              admin.New(admin.Deps{Tx: txManager}),
		CookieSecure:       cfg.CookieSecure,
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	})), nil
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	var h slog.Handler = slog.NewTextHandler(os.Stdout, opts)
	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(middleware.ContextHandler{Handler: h}) // request_id и user_id в каждой строке запроса
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
	poolCfg.ConnConfig.RuntimeParams["application_name"] = dbApplicationName
	warnIfPlaintextRemote(poolCfg)

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
	if err := warnIfSuperuser(pingCtx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	slog.Info("database connected", "max_conns", poolCfg.MaxConns)
	return pool, nil
}

// warnIfSuperuser предупреждает, если DATABASE_URL указывает на суперпользователя.
// Приложение должно ходить под lp_app (backend/db/roles.sql): права БД — последняя линия
// защиты audit_log и сделок. Не падаем, чтобы устаревший .env не ломал запуск.
func warnIfSuperuser(ctx context.Context, pool *pgxpool.Pool) error {
	var superuser bool
	err := pool.QueryRow(ctx, "SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&superuser)
	if err != nil {
		return fmt.Errorf("check database role: %w", err)
	}
	if superuser {
		slog.Warn("DATABASE_URL connects as a superuser; use the lp_app role (see .env.example)")
	}
	return nil
}

// warnIfPlaintextRemote предупреждает о соединении с удалённой БД без TLS (sslmode=disable).
// На стенде sslmode=require/verify-full задаётся в DATABASE_URL.
func warnIfPlaintextRemote(poolCfg *pgxpool.Config) {
	host := poolCfg.ConnConfig.Host
	// Путь — unix-сокет, он не выходит за пределы машины.
	isLocal := host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "/")
	if !isLocal && poolCfg.ConnConfig.TLSConfig == nil {
		slog.Warn("database connection to a remote host is not encrypted; set sslmode=require or verify-full", "host", host)
	}
}

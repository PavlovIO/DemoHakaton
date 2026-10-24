// Package servicetest — общие хелперы интеграционных тестов сервисов: пул тестовой БД,
// транзакция с откатом и минимальные строки фикстур.
package servicetest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// testPasswordHash — заглушка вместо argon2id: пользователи фикстур не входят по паролю.
const testPasswordHash = "x"

// BeginTx открывает транзакцию на тестовой БД и откатывает её в Cleanup, поэтому
// демо-сид не меняется. Без DATABASE_URL или с -short тест пропускается (NewPool).
func BeginTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := NewPool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// UniqueRegistrationNumber — регистрационный номер, не совпадающий с демо-сидом
// (organizations_regnum_uq). UUID короче предела в 50 символов.
func UniqueRegistrationNumber() string {
	return uuid.NewString()
}

// RowQuerier — pgx.Tx или *pgxpool.Pool: фикстуры пишутся и в откатываемую транзакцию, и
// с коммитом (тест конкурентности, BE-09).
type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InsertUser создаёт пользователя с уникальным email; org == nil — у admin.
func InsertUser(ctx context.Context, t *testing.T, tx RowQuerier, role repository.UserRole, org *uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, role, organization_id)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		"test-"+uuid.NewString()+"@example.test", testPasswordHash, role, org).Scan(&id)
	if err != nil {
		t.Fatalf("user %s: %v", role, err)
	}
	return id
}

func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// InsertVerifiedOrg — verified-компания с одним пользователем: заёмщик из RU, кредитор из CY
// (organizations_type_country_chk).
func InsertVerifiedOrg(ctx context.Context, t *testing.T, db RowQuerier, role repository.UserRole, name string) service.Principal {
	t.Helper()
	country := "RU"
	if role == repository.UserRoleLender {
		country = "CY"
	}
	var orgID uuid.UUID
	err := db.QueryRow(ctx, `INSERT INTO organizations (type, name, country_code, registration_number, legal_address,
			kyc_status, verified_at)
		VALUES ($1, $2, $3, $4, 'Адрес', 'verified', now()) RETURNING id`,
		role, "Deals "+name, country, UniqueRegistrationNumber()).Scan(&orgID)
	if err != nil {
		t.Fatalf("organization %s: %v", name, err)
	}
	userID := InsertUser(ctx, t, db, role, &orgID)
	return service.Principal{UserID: userID, OrgID: &orgID, Role: string(role), KycStatus: "verified"}
}

package service_test

// Общие хелперы интеграционных тестов: транзакция с откатом и минимальные строки фикстур.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// testPasswordHash — заглушка вместо argon2id: пользователи фикстур не входят по паролю.
const testPasswordHash = "x"

// beginTestTx открывает транзакцию на тестовой БД и откатывает её в Cleanup, поэтому
// демо-сид не меняется. Без DATABASE_URL или с -short тест пропускается (newPool).
func beginTestTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := newPool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// uniqueRegistrationNumber — регистрационный номер, не совпадающий с демо-сидом
// (organizations_regnum_uq). UUID короче предела в 50 символов.
func uniqueRegistrationNumber() string {
	return uuid.NewString()
}

// insertTestUser создаёт пользователя с уникальным email; org == nil — у admin.
func insertTestUser(ctx context.Context, t *testing.T, tx pgx.Tx, role repository.UserRole, org *uuid.UUID) uuid.UUID {
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

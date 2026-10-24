package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// GetSessionByTokenHash отдаёт kyc_status организации тем же запросом, что и сессию:
// одобрение KYC действует со следующего запроса. У admin организации нет — LEFT JOIN,
// иначе админ не смог бы войти.
func TestSessionKycStatusIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	q := repository.New(tx)

	var orgID uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO organizations
			(type, name, country_code, registration_number, legal_address, kyc_status, verified_at)
		VALUES ('borrower', 'Session KYC', 'RU', $1, 'Адрес', 'verified', now()) RETURNING id`,
		servicetest.UniqueRegistrationNumber()).Scan(&orgID)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	newUser := func(role repository.UserRole, org *uuid.UUID) uuid.UUID {
		return servicetest.InsertUser(ctx, t, tx, role, org)
	}
	newSession := func(userID uuid.UUID) []byte {
		t.Helper()
		hash := make([]byte, sha256.Size) // как sha256(cookie) в auth.Auth
		_, _ = rand.Read(hash)            // с Go 1.24 ошибка всегда nil
		_, err := q.CreateSession(ctx, repository.CreateSessionParams{
			TokenHash: hash,
			UserID:    userID,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatalf("session: %v", err)
		}
		return hash
	}

	borrower, err := q.GetSessionByTokenHash(ctx, newSession(newUser(repository.UserRoleBorrower, &orgID)))
	if err != nil {
		t.Fatalf("borrower session: %v", err)
	}
	want := repository.NullKycStatus{KycStatus: repository.KycStatusVerified, Valid: true}
	if borrower.KycStatus != want {
		t.Errorf("borrower KycStatus = %+v, want %+v", borrower.KycStatus, want)
	}

	admin, err := q.GetSessionByTokenHash(ctx, newSession(newUser(repository.UserRoleAdmin, nil)))
	if err != nil {
		t.Fatalf("admin session (LEFT JOIN organizations): %v", err)
	}
	if admin.KycStatus.Valid {
		t.Errorf("admin KycStatus = %+v, want NULL", admin.KycStatus)
	}
}

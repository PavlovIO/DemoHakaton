package service_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// GetSessionByTokenHash отдаёт kyc_status организации тем же запросом, что и сессию:
// одобрение KYC действует со следующего запроса. У admin организации нет — LEFT JOIN,
// иначе админ не смог бы войти.
func TestSessionKycStatusIntegration(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	q := repository.New(tx)

	var orgID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO organizations
			(type, name, country_code, registration_number, legal_address, kyc_status, verified_at)
		VALUES ('borrower', 'Session KYC', 'RU', $1, 'Адрес', 'verified', now()) RETURNING id`,
		uuid.NewString()[:20]).Scan(&orgID)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	newUser := func(role repository.UserRole, org *uuid.UUID) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, role, organization_id)
			VALUES ($1, 'x', $2, $3) RETURNING id`,
			"session-"+uuid.NewString()+"@example.test", role, org).Scan(&id)
		if err != nil {
			t.Fatalf("user %s: %v", role, err)
		}
		return id
	}
	newSession := func(userID uuid.UUID) []byte {
		t.Helper()
		hash := make([]byte, 32)
		_, _ = rand.Read(hash)
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

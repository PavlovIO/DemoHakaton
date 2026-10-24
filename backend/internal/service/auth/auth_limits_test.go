package auth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	service "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
	"github.com/google/uuid"
)

const (
	limitsEmail    = "limits@example.com"
	limitsPassword = "limitsPassword1"
)

func storeWithUser(t *testing.T) (*fakeAuthStore, uuid.UUID) {
	t.Helper()
	hash, err := auth.HashPassword(context.Background(), limitsPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	store := newFakeAuthStore()
	userID := uuid.New()
	store.users[limitsEmail] = repository.User{
		ID: userID, Email: limitsEmail, PasswordHash: hash,
		Role: repository.UserRoleBorrower, Status: repository.UserStatusActive,
	}
	return store, userID
}

func wantRateLimited(t *testing.T, err error) {
	t.Helper()
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeRateLimited || appErr.RetryAfter <= 0 {
		t.Fatalf("err = %v, want RATE_LIMITED with RetryAfter > 0", err)
	}
}

func TestAuthLoginRateLimit(t *testing.T) {
	limit := auth.DefaultAuthLimits

	t.Run("per email, checked before password", func(t *testing.T) {
		store, _ := storeWithUser(t)
		svc := newTestAuth(store)
		for i := range limit.LoginPerEmail {
			// Разные IP: срабатывает именно лимит на email.
			meta := servicetest.MetaFromIP(fmt.Sprintf("192.0.2.%d", i+1))
			if _, err := svc.Login(context.Background(), limitsEmail, "wrong-password", meta); apperr.CodeOf(err) != apperr.CodeUnauthorized {
				t.Fatalf("attempt %d: err = %v, want unauthorized", i+1, err)
			}
		}
		auditBefore := len(store.audit)

		// Верный пароль не помогает: лимит проверяется до argon2id и до поиска пользователя.
		_, err := svc.Login(context.Background(), " LIMITS@example.com ", limitsPassword, servicetest.MetaFromIP("198.51.100.1"))
		wantRateLimited(t, err)
		if len(store.audit) != auditBefore {
			t.Fatalf("rate-limited attempt wrote audit: %d -> %d", auditBefore, len(store.audit))
		}
	})

	t.Run("success resets email counter", func(t *testing.T) {
		store, _ := storeWithUser(t)
		svc := newTestAuth(store)
		meta := servicetest.MetaFromIP("192.0.2.1")
		for range limit.LoginPerEmail - 1 {
			_, _ = svc.Login(context.Background(), limitsEmail, "wrong-password", meta)
		}
		if _, err := svc.Login(context.Background(), limitsEmail, limitsPassword, meta); err != nil {
			t.Fatalf("Login() with correct password err = %v", err)
		}
		for i := range limit.LoginPerEmail {
			if _, err := svc.Login(context.Background(), limitsEmail, "wrong-password", meta); apperr.CodeOf(err) != apperr.CodeUnauthorized {
				t.Fatalf("attempt %d after success: err = %v, want unauthorized", i+1, err)
			}
		}
	})

	t.Run("per IP", func(t *testing.T) {
		store, _ := storeWithUser(t)
		svc := newTestAuth(store)
		meta := servicetest.MetaFromIP("192.0.2.7")
		for i := range limit.LoginPerIP {
			email := fmt.Sprintf("user%d@example.com", i)
			if _, err := svc.Login(context.Background(), email, "wrong-password", meta); apperr.CodeOf(err) != apperr.CodeUnauthorized {
				t.Fatalf("attempt %d: err = %v, want unauthorized", i+1, err)
			}
		}
		_, err := svc.Login(context.Background(), limitsEmail, limitsPassword, meta)
		wantRateLimited(t, err)
	})

	t.Run("window expires", func(t *testing.T) {
		store, _ := storeWithUser(t)
		now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		svc := newTestAuthWithLimiter(store, service.NewMemoryRateLimiterWithClock(func() time.Time { return now }))
		meta := servicetest.MetaFromIP("192.0.2.1")
		for range limit.LoginPerEmail {
			_, _ = svc.Login(context.Background(), limitsEmail, "wrong-password", meta)
		}
		_, err := svc.Login(context.Background(), limitsEmail, limitsPassword, meta)
		wantRateLimited(t, err)

		now = now.Add(limit.LoginWindow + time.Second)
		if _, err := svc.Login(context.Background(), limitsEmail, limitsPassword, meta); err != nil {
			t.Fatalf("Login() after window err = %v", err)
		}
	})
}

func TestAuthRegisterRateLimit(t *testing.T) {
	store := newFakeAuthStore()
	svc := newTestAuth(store)
	meta := servicetest.MetaFromIP("192.0.2.9")
	register := func(i int) error {
		_, err := svc.Register(context.Background(), auth.RegisterInput{
			Email: fmt.Sprintf("reg%d@example.com", i), Password: "1234567890", Role: "borrower",
			Org: auth.RegisterOrganizationInput{Name: "Org", CountryCode: "RU", RegistrationNumber: fmt.Sprintf("reg-%d", i), LegalAddress: "addr"},
		}, meta)
		return err
	}
	for i := range auth.DefaultAuthLimits.RegisterPerIP {
		if err := register(i); err != nil {
			t.Fatalf("registration %d err = %v", i+1, err)
		}
	}
	wantRateLimited(t, register(auth.DefaultAuthLimits.RegisterPerIP))
}

func TestAuthLoginRollsBackOnSessionFailure(t *testing.T) {
	store, userID := storeWithUser(t)
	store.sessionErr = errors.New("insert session failed")
	svc := newTestAuth(store)

	session, err := svc.Login(context.Background(), limitsEmail, limitsPassword, servicetest.MetaFromIP("192.0.2.1"))
	if err == nil || session != nil {
		t.Fatalf("Login() = (%v, %v), want error", session, err)
	}
	if store.lastLogin[userID] {
		t.Fatal("UpdateLastLogin was not rolled back")
	}
	for _, entry := range store.audit {
		if entry.Action == "auth.login" {
			t.Fatal("auth.login audit entry survived rollback")
		}
	}
}

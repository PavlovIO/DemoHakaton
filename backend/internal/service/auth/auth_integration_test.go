package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// Истечение и блокировку проверяет SQL (GetSessionByTokenHash), поэтому тест — на настоящей БД.
// Пользователи не удаляются: после DB-SEC у lp_app нет DELETE на users; суффикс делает их уникальными.
func TestAuthIntegrationSessionLifecycle(t *testing.T) {
	pool := servicetest.NewPool(t)
	ctx := context.Background()
	svc := auth.NewAuth(repository.New(pool), service.NewTxManager(pool), time.Hour,
		service.NewMemoryRateLimiter(), auth.DefaultAuthLimits)

	suffix := uuid.NewString()[:8]
	email := "lifecycle-" + suffix + "@example.test"
	const password = "lifecyclePassword1"
	session, err := svc.Register(ctx, auth.RegisterInput{
		Email: email, Password: password, Role: "lender",
		Org: auth.RegisterOrganizationInput{
			Name: "Lifecycle " + suffix, CountryCode: "CY", RegistrationNumber: "LC-" + suffix, LegalAddress: "Limassol",
		},
	}, service.ClientMeta{UserAgent: "integration"})
	if err != nil {
		t.Fatalf("Register() err = %v", err)
	}
	userID := session.Me.User.ID

	lastSeen := func() time.Time {
		t.Helper()
		var seen time.Time
		if err := pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE user_id = $1`, userID).Scan(&seen); err != nil {
			t.Fatalf("last_seen_at: %v", err)
		}
		return seen
	}
	wantUnauthorized := func(token, why string) {
		t.Helper()
		if _, err := svc.Authenticate(ctx, token); apperr.CodeOf(err) != apperr.CodeUnauthorized {
			t.Fatalf("Authenticate() %s: err = %v, want UNAUTHORIZED", why, err)
		}
	}

	t.Run("valid session touches last_seen_at", func(t *testing.T) {
		before := lastSeen()
		time.Sleep(10 * time.Millisecond)
		p, err := svc.Authenticate(ctx, session.Token)
		if err != nil || p.UserID != userID {
			t.Fatalf("Authenticate() = (%+v, %v), want principal of %s", p, err, userID)
		}
		if after := lastSeen(); !after.After(before) {
			t.Fatalf("last_seen_at = %s, want after %s", after, before)
		}
	})

	t.Run("expired session", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 second' WHERE user_id = $1`, userID); err != nil {
			t.Fatalf("expire session: %v", err)
		}
		wantUnauthorized(session.Token, "after expiry")
	})

	t.Run("blocked user", func(t *testing.T) {
		fresh, err := svc.Login(ctx, email, password, service.ClientMeta{UserAgent: "integration"})
		if err != nil {
			t.Fatalf("Login() err = %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE users SET status = 'blocked' WHERE id = $1`, userID); err != nil {
			t.Fatalf("block user: %v", err)
		}
		wantUnauthorized(fresh.Token, "of blocked user")
		if _, err := pool.Exec(ctx, `UPDATE users SET status = 'active' WHERE id = $1`, userID); err != nil {
			t.Fatalf("unblock user: %v", err)
		}
	})

	t.Run("logout deletes session", func(t *testing.T) {
		fresh, err := svc.Login(ctx, email, password, service.ClientMeta{UserAgent: "integration"})
		if err != nil {
			t.Fatalf("Login() err = %v", err)
		}
		count := func() int {
			t.Helper()
			var n int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash = sha256($1::bytea)`, fresh.Token).Scan(&n); err != nil {
				t.Fatalf("count sessions: %v", err)
			}
			return n
		}
		if n := count(); n != 1 {
			t.Fatalf("sessions before logout = %d, want 1", n)
		}
		if err := svc.Logout(ctx, fresh.Token); err != nil {
			t.Fatalf("Logout() err = %v", err)
		}
		if n := count(); n != 0 {
			t.Fatalf("sessions after logout = %d, want 0", n)
		}
		wantUnauthorized(fresh.Token, "after logout")
	})
}

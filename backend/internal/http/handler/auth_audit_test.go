package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
)

// auditAuthStore — хранилище настоящего auth.Auth для неудачного входа: пользователя
// нет, записи аудита запоминаются или падают с auditErr. Остальные методы не вызываются.
type auditAuthStore struct {
	auth.AuthStore
	audit    []repository.CreateAuditLogParams
	auditErr error
}

func (s *auditAuthStore) GetUserByEmail(context.Context, string) (repository.User, error) {
	return repository.User{}, pgx.ErrNoRows
}

func (s *auditAuthStore) CreateAuditLog(_ context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error) {
	if s.auditErr != nil {
		return repository.AuditLog{}, s.auditErr
	}
	s.audit = append(s.audit, arg)
	return repository.AuditLog{}, nil
}

// failedLogin — POST /auth/login с незнакомым email через роутер с настоящим auth.Auth.
func failedLogin(t *testing.T, store *auditAuthStore, requestID string) {
	t.Helper()
	svc := auth.NewAuth(store, nil, time.Hour, service.NewMemoryRateLimiter(), auth.DefaultAuthLimits)
	srv := newTestServer(t, handler.Deps{Auth: svc})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login",
		strings.NewReader(`{"email":"nobody@example.com","password":"password1234"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "audit-http-agent")
	req.Header.Set("X-Request-ID", requestID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestFailedLoginAuditHasClientMeta(t *testing.T) {
	store := &auditAuthStore{}
	failedLogin(t, store, "login-meta")

	if len(store.audit) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(store.audit))
	}
	entry := store.audit[0]
	if entry.Action != auth.AuditAuthLoginFailed {
		t.Errorf("action = %q, want %q", entry.Action, auth.AuditAuthLoginFailed)
	}
	if entry.Ip == nil || !entry.Ip.IsLoopback() {
		t.Errorf("ip = %v, want loopback адрес клиента", entry.Ip)
	}
	if entry.UserAgent.String != "audit-http-agent" {
		t.Errorf("user_agent = %q, want audit-http-agent", entry.UserAgent.String)
	}
	var meta map[string]any
	if err := json.Unmarshal(entry.Meta, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["reason"] != "unknown_email" || meta["email"] != nil {
		t.Errorf("meta = %v, want reason=unknown_email без email", meta)
	}
}

// Сбой записи аудита не меняет ответ (401), а строка лога связана с запросом.
func TestFailedLoginAuditWriteErrorLogged(t *testing.T) {
	logs := captureLogs(t)
	failedLogin(t, &auditAuthStore{auditErr: errors.New("db down")}, "login-audit-fail")

	line := findLine(logs.lines(t), "audit write failed")
	if line == nil {
		t.Fatal("нет строки audit write failed")
	}
	if line["request_id"] != "login-audit-fail" || line["action"] != auth.AuditAuthLoginFailed || line["level"] != "WARN" {
		t.Errorf("строка лога = %v", line)
	}
}

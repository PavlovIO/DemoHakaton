package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	service "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeAuthStore хранит состояние аутентификации для unit-тестов без БД.
type fakeAuthStore struct {
	repository.Querier
	users     map[string]repository.User
	orgs      map[uuid.UUID]repository.Organization
	sessions  map[string]repository.GetSessionByTokenHashRow
	deleted   [][]byte
	lastLogin map[uuid.UUID]bool
	audit     []repository.CreateAuditLogParams
	auditErr  error
	// sessionErr — ошибка CreateSession (проверка отката Login).
	sessionErr error
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		users:     map[string]repository.User{},
		orgs:      map[uuid.UUID]repository.Organization{},
		sessions:  map[string]repository.GetSessionByTokenHashRow{},
		lastLogin: map[uuid.UUID]bool{},
	}
}

func (s *fakeAuthStore) GetUserByEmail(ctx context.Context, email string) (repository.User, error) {
	if u, ok := s.users[email]; ok {
		return u, nil
	}
	return repository.User{}, pgx.ErrNoRows
}

func (s *fakeAuthStore) GetUserByID(ctx context.Context, id uuid.UUID) (repository.User, error) {
	for _, u := range s.users {
		if u.ID == id {
			return u, nil
		}
	}
	return repository.User{}, pgx.ErrNoRows
}

func (s *fakeAuthStore) CreateUser(ctx context.Context, arg repository.CreateUserParams) (repository.CreateUserRow, error) {
	if _, exists := s.users[arg.Email]; exists {
		return repository.CreateUserRow{}, &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	}
	id := uuid.New()
	now := time.Now()
	u := repository.User{
		ID:             id,
		Email:          arg.Email,
		PasswordHash:   arg.PasswordHash,
		Role:           arg.Role,
		Status:         repository.UserStatusActive,
		OrganizationID: arg.OrganizationID,
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}
	s.users[arg.Email] = u
	return repository.CreateUserRow{ID: id, Email: arg.Email, Role: arg.Role, Status: repository.UserStatusActive, OrganizationID: arg.OrganizationID, CreatedAt: u.CreatedAt}, nil
}

func (s *fakeAuthStore) CreateOrganization(ctx context.Context, arg repository.CreateOrganizationParams) (repository.Organization, error) {
	for _, o := range s.orgs {
		if o.RegistrationNumber == arg.RegistrationNumber {
			return repository.Organization{}, &pgconn.PgError{Code: "23505", ConstraintName: "organizations_regnum_uq"}
		}
	}
	id := uuid.New()
	now := time.Now()
	org := repository.Organization{
		ID:                 id,
		Type:               arg.Type,
		Name:               arg.Name,
		CountryCode:        arg.CountryCode,
		RegistrationNumber: arg.RegistrationNumber,
		Lei:                arg.Lei,
		LegalAddress:       arg.LegalAddress,
		KycStatus:          repository.KycStatusDraft,
		CreatedAt:          pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:          pgtype.Timestamptz{Time: now, Valid: true},
	}
	s.orgs[id] = org
	return org, nil
}

func (s *fakeAuthStore) GetOrganizationByID(ctx context.Context, id uuid.UUID) (repository.Organization, error) {
	if org, ok := s.orgs[id]; ok {
		return org, nil
	}
	return repository.Organization{}, pgx.ErrNoRows
}

func (s *fakeAuthStore) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	s.lastLogin[id] = true
	return nil
}

func (s *fakeAuthStore) CreateSession(ctx context.Context, arg repository.CreateSessionParams) (repository.CreateSessionRow, error) {
	if s.sessionErr != nil {
		return repository.CreateSessionRow{}, s.sessionErr
	}
	var user repository.User
	for _, u := range s.users {
		if u.ID == arg.UserID {
			user = u
			break
		}
	}
	if user.ID == uuid.Nil {
		return repository.CreateSessionRow{}, pgx.ErrNoRows
	}
	row := repository.GetSessionByTokenHashRow{
		TokenHash:      arg.TokenHash,
		UserID:         arg.UserID,
		ExpiresAt:      arg.ExpiresAt,
		CreatedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Email:          user.Email,
		Role:           user.Role,
		Status:         user.Status,
		OrganizationID: user.OrganizationID,
	}
	s.sessions[string(arg.TokenHash)] = row
	return repository.CreateSessionRow{TokenHash: arg.TokenHash, UserID: arg.UserID, ExpiresAt: arg.ExpiresAt, CreatedAt: row.CreatedAt}, nil
}

func (s *fakeAuthStore) CreateAuditLog(_ context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error) {
	s.audit = append(s.audit, arg)
	return repository.AuditLog{}, s.auditErr
}

func (s *fakeAuthStore) GetSessionByTokenHash(ctx context.Context, tokenHash []byte) (repository.GetSessionByTokenHashRow, error) {
	if row, ok := s.sessions[string(tokenHash)]; ok {
		return row, nil
	}
	return repository.GetSessionByTokenHashRow{}, pgx.ErrNoRows
}

func (s *fakeAuthStore) TouchSession(ctx context.Context, tokenHash []byte) error {
	if row, ok := s.sessions[string(tokenHash)]; ok {
		row.LastSeenAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		s.sessions[string(tokenHash)] = row
	}
	return nil
}

func (s *fakeAuthStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash []byte) error {
	s.deleted = append(s.deleted, append([]byte(nil), tokenHash...))
	delete(s.sessions, string(tokenHash))
	return nil
}

// fakeAuthTx симулирует транзакцию: callback работает на заглушке хранилища,
// при ошибке её состояние откатывается к снимку до вызова.
type fakeAuthTx struct {
	store *fakeAuthStore
}

func (tx *fakeAuthTx) InTx(ctx context.Context, fn func(q repository.Querier) error) error {
	s := tx.store
	users, orgs, sessions := maps.Clone(s.users), maps.Clone(s.orgs), maps.Clone(s.sessions)
	lastLogin, audit := maps.Clone(s.lastLogin), slices.Clone(s.audit)
	if err := fn(s); err != nil {
		s.users, s.orgs, s.sessions, s.lastLogin, s.audit = users, orgs, sessions, lastLogin, audit
		return err
	}
	return nil
}

func newTestAuth(store *fakeAuthStore) *service.Auth {
	return newTestAuthWithLimiter(store, service.NewMemoryRateLimiter())
}

func newTestAuthWithLimiter(store *fakeAuthStore, limiter service.RateLimiter) *service.Auth {
	return service.NewAuth(store, &fakeAuthTx{store: store}, time.Hour, limiter, service.DefaultAuthLimits)
}

func hashOfToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func TestAuthRegister(t *testing.T) {
	cases := []struct {
		name        string
		input       service.RegisterInput
		wantErr     bool
		wantErrCode apperr.Code
	}{
		{name: "borrower ru ok", input: service.RegisterInput{Email: "  User@Example.com ", Password: "1234567890", Role: "borrower", Org: service.RegisterOrganizationInput{Name: "ACME", CountryCode: "ru", RegistrationNumber: "R-1", LegalAddress: "addr"}}, wantErr: false},
		{name: "borrower cy invalid", input: service.RegisterInput{Email: "u@example.com", Password: "1234567890", Role: "borrower", Org: service.RegisterOrganizationInput{Name: "ACME", CountryCode: "CY", RegistrationNumber: "R-2", LegalAddress: "addr"}}, wantErr: true, wantErrCode: apperr.CodeValidation},
		{name: "lender ru invalid", input: service.RegisterInput{Email: "u@example.com", Password: "1234567890", Role: "lender", Org: service.RegisterOrganizationInput{Name: "ACME", CountryCode: "RU", RegistrationNumber: "R-3", LegalAddress: "addr"}}, wantErr: true, wantErrCode: apperr.CodeValidation},
		{name: "role admin invalid", input: service.RegisterInput{Email: "u@example.com", Password: "1234567890", Role: "admin", Org: service.RegisterOrganizationInput{Name: "ACME", CountryCode: "US", RegistrationNumber: "R-4", LegalAddress: "addr"}}, wantErr: true, wantErrCode: apperr.CodeValidation},
		{name: "short password invalid", input: service.RegisterInput{Email: "u@example.com", Password: "123456789", Role: "lender", Org: service.RegisterOrganizationInput{Name: "ACME", CountryCode: "US", RegistrationNumber: "R-5", LegalAddress: "addr"}}, wantErr: true, wantErrCode: apperr.CodeValidation},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeAuthStore()
			auth := newTestAuth(store)
			session, err := auth.Register(context.Background(), tc.input, service.ClientMeta{UserAgent: "ua"})
			if tc.wantErr {
				if err == nil {
					t.Fatal("Register() err == nil, want validation error")
				}
				appErr, ok := apperr.As(err)
				if !ok || appErr.Code != tc.wantErrCode {
					t.Fatalf("Register() err = %v, want code %s", err, tc.wantErrCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("Register() err = %v", err)
			}
			if session == nil || session.Token == "" || session.CSRFToken == "" {
				t.Fatalf("Register() returned empty session tokens")
			}
			if session.Me == nil || session.Me.User.Email != strings.ToLower(strings.TrimSpace(tc.input.Email)) {
				t.Fatalf("Register() email = %q, want %q", session.Me.User.Email, strings.ToLower(strings.TrimSpace(tc.input.Email)))
			}
		})
	}

	t.Run("duplicate email conflict", func(t *testing.T) {
		store := newFakeAuthStore()
		auth := newTestAuth(store)
		_, err := auth.Register(context.Background(), service.RegisterInput{Email: "dup@example.com", Password: "1234567890", Role: "borrower", Org: service.RegisterOrganizationInput{Name: "A", CountryCode: "RU", RegistrationNumber: "dup-1", LegalAddress: "addr"}}, service.ClientMeta{})
		if err != nil {
			t.Fatalf("first registration error = %v", err)
		}
		_, err = auth.Register(context.Background(), service.RegisterInput{Email: "dup@example.com", Password: "1234567890", Role: "lender", Org: service.RegisterOrganizationInput{Name: "B", CountryCode: "US", RegistrationNumber: "dup-2", LegalAddress: "addr"}}, service.ClientMeta{})
		appErr, ok := apperr.As(err)
		if !ok || appErr.Code != apperr.CodeConflict || appErr.Message != "Пользователь с таким email уже существует" {
			t.Fatalf("second registration err = %v, want conflict about email", err)
		}
	})

	t.Run("duplicate registration number conflict", func(t *testing.T) {
		store := newFakeAuthStore()
		auth := newTestAuth(store)
		in := service.RegisterInput{Email: "a@example.com", Password: "1234567890", Role: "borrower", Org: service.RegisterOrganizationInput{Name: "A", CountryCode: "RU", RegistrationNumber: "same", LegalAddress: "addr"}}
		if _, err := auth.Register(context.Background(), in, service.ClientMeta{}); err != nil {
			t.Fatalf("first registration error = %v", err)
		}
		in.Email = "b@example.com"
		_, err := auth.Register(context.Background(), in, service.ClientMeta{})
		appErr, ok := apperr.As(err)
		if !ok || appErr.Code != apperr.CodeConflict || appErr.Message != "Организация с таким номером уже зарегистрирована" {
			t.Fatalf("second registration err = %v, want conflict about organization", err)
		}
	})

	t.Run("session stores hash and role", func(t *testing.T) {
		store := newFakeAuthStore()
		auth := newTestAuth(store)
		input := service.RegisterInput{Email: "role@example.com", Password: "1234567890", Role: "lender", Org: service.RegisterOrganizationInput{Name: "Org", CountryCode: "US", RegistrationNumber: "reg-1", LegalAddress: "addr"}}
		session, err := auth.Register(context.Background(), input, service.ClientMeta{UserAgent: "ua"})
		if err != nil {
			t.Fatalf("Register() err = %v", err)
		}
		if session.Me == nil || session.Me.Organization == nil {
			t.Fatal("Register() returned session without organization")
		}
		if session.Me.User.Role != "lender" {
			t.Fatalf("Register() user role = %q, want %q", session.Me.User.Role, "lender")
		}
		if session.Me.Organization.ID == uuid.Nil {
			t.Fatal("Register() organization id is zero")
		}
		if _, ok := store.sessions[string(hashOfToken(session.Token))]; !ok {
			t.Fatal("Register() saved session under token hash, not raw token")
		}
		if _, ok := store.sessions[session.Token]; ok {
			t.Fatal("Register() saved raw session token as key")
		}
	})
}

func TestAuthLogin(t *testing.T) {
	store := newFakeAuthStore()
	auth := newTestAuth(store)
	const (
		email    = "login@example.com"
		password = "secretPassword1"
	)
	_, err := auth.Register(context.Background(), service.RegisterInput{Email: email, Password: password, Role: "borrower", Org: service.RegisterOrganizationInput{Name: "A", CountryCode: "RU", RegistrationNumber: "login-reg", LegalAddress: "addr"}}, service.ClientMeta{UserAgent: "ua"})
	if err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	_, err = auth.Login(context.Background(), email, "wrong-password", service.ClientMeta{UserAgent: "ua"})
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeUnauthorized || appErr.Message != "Неверный email или пароль" {
		t.Fatalf("Login() wrong password err = %v, want unauthorized message", err)
	}

	_, err = auth.Login(context.Background(), "missing@example.com", password, service.ClientMeta{UserAgent: "ua"})
	appErr, ok = apperr.As(err)
	if !ok || appErr.Code != apperr.CodeUnauthorized || appErr.Message != "Неверный email или пароль" {
		t.Fatalf("Login() missing user err = %v, want same unauthorized message", err)
	}

	user, err := store.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("GetUserByEmail() err = %v", err)
	}
	user.Status = repository.UserStatusBlocked
	store.users[email] = user
	_, err = auth.Login(context.Background(), email, password, service.ClientMeta{UserAgent: "ua"})
	appErr, ok = apperr.As(err)
	if !ok || appErr.Code != apperr.CodeForbidden {
		t.Fatalf("Login() blocked user err = %v, want forbidden", err)
	}

	_, err = auth.Login(context.Background(), email, "wrong-password", service.ClientMeta{UserAgent: "ua"})
	appErr, ok = apperr.As(err)
	if !ok || appErr.Code != apperr.CodeUnauthorized {
		t.Fatalf("Login() wrong password for blocked user err = %v, want unauthorized", err)
	}

	user.Status = repository.UserStatusActive
	store.users[email] = user
	session, err := auth.Login(context.Background(), email, password, service.ClientMeta{UserAgent: "ua"})
	if err != nil {
		t.Fatalf("Login() success err = %v", err)
	}
	if session == nil || session.Token == "" {
		t.Fatal("Login() success returned empty token")
	}
	if _, ok := store.sessions[string(hashOfToken(session.Token))]; !ok {
		t.Fatal("Login() stored session using raw token instead of hash")
	}
	if _, ok := store.sessions[session.Token]; ok {
		t.Fatal("Login() stored raw token as session key")
	}
}

func TestAuthLoginAudit(t *testing.T) {
	const (
		email    = "audit-login@example.com"
		password = "auditSecretPassword1"
	)
	ip := netip.MustParseAddr("192.0.2.42")
	meta := service.ClientMeta{IP: &ip, UserAgent: "auth-test-agent"}

	passwordHash, err := service.HashPassword(context.Background(), password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	cases := []struct {
		name          string
		loginEmail    string
		loginPassword string
		status        repository.UserStatus
		wantAction    string
		wantReason    string
		wantUserID    bool
		wantErrCode   apperr.Code
	}{
		{name: "success", loginEmail: email, loginPassword: password, status: repository.UserStatusActive, wantAction: "auth.login", wantUserID: true},
		{name: "bad password", loginEmail: email, loginPassword: "incorrect-password", status: repository.UserStatusActive, wantAction: "auth.login_failed", wantReason: "bad_password", wantUserID: true, wantErrCode: apperr.CodeUnauthorized},
		{name: "unknown email", loginEmail: "unknown@example.com", loginPassword: password, status: repository.UserStatusActive, wantAction: "auth.login_failed", wantReason: "unknown_email", wantErrCode: apperr.CodeUnauthorized},
		{name: "blocked", loginEmail: email, loginPassword: password, status: repository.UserStatusBlocked, wantAction: "auth.login_failed", wantReason: "blocked", wantUserID: true, wantErrCode: apperr.CodeForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeAuthStore()
			userID := uuid.New()
			store.users[email] = repository.User{
				ID:           userID,
				Email:        email,
				PasswordHash: passwordHash,
				Role:         repository.UserRoleBorrower,
				Status:       tc.status,
			}
			auth := newTestAuth(store)

			session, loginErr := auth.Login(context.Background(), tc.loginEmail, tc.loginPassword, meta)
			if tc.wantErrCode == "" {
				if loginErr != nil || session == nil || session.Token == "" {
					t.Fatalf("Login() = (%v, %v), want session", session, loginErr)
				}
			} else if appErr, ok := apperr.As(loginErr); !ok || appErr.Code != tc.wantErrCode {
				t.Fatalf("Login() error = %v, want code %s", loginErr, tc.wantErrCode)
			}
			if len(store.audit) != 1 {
				t.Fatalf("audit entries = %d, want exactly 1", len(store.audit))
			}
			entry := store.audit[0]
			if entry.Action != tc.wantAction {
				t.Errorf("audit action = %q, want %q", entry.Action, tc.wantAction)
			}
			if entry.UserID.Valid != tc.wantUserID {
				t.Errorf("audit UserID.Valid = %v, want %v", entry.UserID.Valid, tc.wantUserID)
			}
			if tc.wantUserID && uuid.UUID(entry.UserID.Bytes) != userID {
				t.Errorf("audit user id = %s, want %s", uuid.UUID(entry.UserID.Bytes), userID)
			}
			if entry.EntityType.Valid != tc.wantUserID {
				t.Errorf("audit EntityType.Valid = %v, want %v", entry.EntityType.Valid, tc.wantUserID)
			}
			if tc.wantUserID {
				if entry.EntityType.EntityType != repository.EntityTypeUser {
					t.Errorf("audit entity type = %q, want user", entry.EntityType.EntityType)
				}
				if !entry.EntityID.Valid || entry.EntityID.String != userID.String() {
					t.Errorf("audit entity id = %+v, want %s", entry.EntityID, userID)
				}
			} else if entry.EntityID.Valid {
				t.Errorf("audit entity id = %+v, want NULL", entry.EntityID)
			}
			if entry.Ip == nil || *entry.Ip != ip {
				t.Errorf("audit IP = %v, want %v", entry.Ip, ip)
			}
			if !entry.UserAgent.Valid || entry.UserAgent.String != meta.UserAgent {
				t.Errorf("audit user agent = %+v, want %q", entry.UserAgent, meta.UserAgent)
			}
			var gotMeta map[string]string
			if len(entry.Meta) == 0 || string(entry.Meta) == "null" {
				t.Fatalf("audit meta = %s, want non-null JSON", entry.Meta)
			}
			if err := json.Unmarshal(entry.Meta, &gotMeta); err != nil {
				t.Fatalf("decode audit meta %s: %v", entry.Meta, err)
			}
			if tc.wantReason != "" {
				if gotMeta["reason"] != tc.wantReason {
					t.Errorf("audit reason = %q, want %q", gotMeta["reason"], tc.wantReason)
				}
			} else if string(entry.Meta) != "{}" {
				t.Errorf("audit meta = %s, want {}", entry.Meta)
			}
			if strings.Contains(string(entry.Meta), password) || strings.Contains(string(entry.Meta), email) {
				t.Errorf("audit meta contains credentials: %s", entry.Meta)
			}
		})
	}

	// auth.login пишется в транзакции входа: его сбой откатывает вход.
	// auth.login_failed — best-effort: сбой записи не меняет ответ.
	t.Run("audit failures", func(t *testing.T) {
		for _, tc := range []struct {
			name          string
			loginPassword string
			wantCode      apperr.Code
		}{
			{name: "success rolls back", loginPassword: password, wantCode: apperr.CodeInternal},
			{name: "bad password", loginPassword: "incorrect-password", wantCode: apperr.CodeUnauthorized},
		} {
			t.Run(tc.name, func(t *testing.T) {
				store := newFakeAuthStore()
				userID := uuid.New()
				store.users[email] = repository.User{
					ID:           userID,
					Email:        email,
					PasswordHash: passwordHash,
					Role:         repository.UserRoleBorrower,
					Status:       repository.UserStatusActive,
				}
				store.auditErr = errors.New("audit storage unavailable")
				auth := newTestAuth(store)

				session, loginErr := auth.Login(context.Background(), email, tc.loginPassword, meta)
				if session != nil || apperr.CodeOf(loginErr) != tc.wantCode {
					t.Fatalf("Login() = (%v, %v), want error with code %s", session, loginErr, tc.wantCode)
				}
				if len(store.sessions) != 0 || store.lastLogin[userID] {
					t.Fatalf("failed login left state: sessions=%d lastLogin=%v", len(store.sessions), store.lastLogin[userID])
				}
			})
		}
	})
}

func TestAuthAuthenticateAndLogout(t *testing.T) {
	store := newFakeAuthStore()
	auth := newTestAuth(store)
	session, err := auth.Register(context.Background(), service.RegisterInput{Email: "auth@example.com", Password: "1234567890", Role: "lender", Org: service.RegisterOrganizationInput{Name: "Org", CountryCode: "US", RegistrationNumber: "auth-reg", LegalAddress: "Address"}}, service.ClientMeta{UserAgent: "ua"})
	if err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if _, err = auth.Authenticate(context.Background(), ""); err == nil {
		t.Fatal("Authenticate() with empty token should fail")
	}
	if _, err = auth.Authenticate(context.Background(), "unknown-token"); err == nil {
		t.Fatal("Authenticate() with unknown token should fail")
	}

	principal, err := auth.Authenticate(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("Authenticate() valid token err = %v", err)
	}
	if principal == nil || principal.UserID == uuid.Nil {
		t.Fatal("Authenticate() returned empty principal")
	}
	if principal.OrgID == nil || *principal.OrgID != session.Me.Organization.ID {
		t.Fatal("Authenticate() principal has unexpected org id")
	}
	if principal.Role != "lender" {
		t.Fatalf("Authenticate() principal role = %q, want %q", principal.Role, "lender")
	}

	if err := auth.Logout(context.Background(), session.Token); err != nil {
		t.Fatalf("Logout() err = %v", err)
	}
	if _, err := auth.Authenticate(context.Background(), session.Token); err == nil {
		t.Fatal("Authenticate() after logout should fail")
	}
}

// TestAuthAuthenticateKycStatus: KYC-статус компании приходит в Principal из той же строки
// сессии, а у admin (без организации) он пустой и ViewerOrg — uuid.Nil.
func TestAuthAuthenticateKycStatus(t *testing.T) {
	orgID := uuid.New()
	tests := []struct {
		name          string
		row           repository.GetSessionByTokenHashRow
		wantKyc       string
		wantOrg       *uuid.UUID
		wantRole      repository.UserRole
		wantViewerOrg uuid.UUID
	}{
		{
			name: "borrower verified",
			row: repository.GetSessionByTokenHashRow{
				UserID:         uuid.New(),
				Role:           repository.UserRoleBorrower,
				OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true},
				KycStatus:      repository.NullKycStatus{KycStatus: repository.KycStatusVerified, Valid: true},
			},
			wantKyc:       "verified",
			wantOrg:       &orgID,
			wantRole:      repository.UserRoleBorrower,
			wantViewerOrg: orgID,
		},
		{
			name: "admin",
			row: repository.GetSessionByTokenHashRow{
				UserID: uuid.New(),
				Role:   repository.UserRoleAdmin,
			},
			wantKyc:       "",
			wantRole:      repository.UserRoleAdmin,
			wantViewerOrg: uuid.Nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeAuthStore()
			const token = "session-token"
			sum := sha256.Sum256([]byte(token))
			store.sessions[string(sum[:])] = tc.row
			auth := newTestAuth(store)

			p, err := auth.Authenticate(context.Background(), token)
			if err != nil {
				t.Fatalf("Authenticate() err = %v", err)
			}
			if p.KycStatus != tc.wantKyc {
				t.Errorf("KycStatus = %q, want %q", p.KycStatus, tc.wantKyc)
			}
			if (p.OrgID == nil) != (tc.wantOrg == nil) || (p.OrgID != nil && *p.OrgID != *tc.wantOrg) {
				t.Errorf("OrgID = %v, want %v", p.OrgID, tc.wantOrg)
			}
			if p.ViewerRole() != tc.wantRole {
				t.Errorf("ViewerRole() = %q, want %q", p.ViewerRole(), tc.wantRole)
			}
			if p.ViewerOrg() != tc.wantViewerOrg {
				t.Errorf("ViewerOrg() = %v, want %v", p.ViewerOrg(), tc.wantViewerOrg)
			}
		})
	}
}

func TestAuthMe(t *testing.T) {
	store := newFakeAuthStore()
	auth := newTestAuth(store)
	session, err := auth.Register(context.Background(), service.RegisterInput{Email: "me@example.com", Password: "1234567890", Role: "borrower", Org: service.RegisterOrganizationInput{Name: "Org", CountryCode: "RU", RegistrationNumber: "me-reg", LegalAddress: "Address"}}, service.ClientMeta{})
	if err != nil {
		t.Fatalf("Register() err = %v", err)
	}
	principal, err := auth.Authenticate(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("Authenticate() err = %v", err)
	}
	me, err := auth.Me(context.Background(), *principal)
	if err != nil {
		t.Fatalf("Me() err = %v", err)
	}
	if me == nil || me.User.Email == "" {
		t.Fatal("Me() returned empty payload")
	}
	if me.Organization == nil || me.Organization.Name == "" {
		t.Fatal("Me() missing organization")
	}

	adminID := uuid.New()
	store.users["admin@example.com"] = repository.User{
		ID:           adminID,
		Email:        "admin@example.com",
		PasswordHash: "hash",
		Role:         repository.UserRoleAdmin,
		Status:       repository.UserStatusActive,
		CreatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	meAdmin, err := auth.Me(context.Background(), service.Principal{UserID: adminID, Role: "admin"})
	if err != nil {
		t.Fatalf("Me() admin err = %v", err)
	}
	if meAdmin.Organization != nil {
		t.Fatal("Me() for admin should return nil organization")
	}
}

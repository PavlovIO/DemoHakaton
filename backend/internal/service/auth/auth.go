package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/pgconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Действия audit_log аутентификации (BE-14). auth.logout не пишется (DECISIONS п. 16).
const (
	AuditAuthLogin       = "auth.login"
	AuditAuthLoginFailed = "auth.login_failed"
	AuditAuthRegister    = "auth.register"
)

// RegisterOrganizationInput описывает организацию при регистрации пользователя.
type RegisterOrganizationInput struct {
	Name               string
	CountryCode        string
	RegistrationNumber string
	LEI                *string
	LegalAddress       string
}

// RegisterInput содержит входные данные регистрации нового пользователя.
type RegisterInput struct {
	Email    string
	Password string
	Role     string
	Org      RegisterOrganizationInput
}

// Session хранит данные созданной пользовательской сессии.
type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	Me        *Me
}

// User — доменная модель пользователя для выдачи в контекст и в `Me`.
type User struct {
	ID             uuid.UUID
	Email          string
	Role           string
	Status         string
	OrganizationID *uuid.UUID
	LastLoginAt    *time.Time
	CreatedAt      time.Time
}

// Me — ответ сервиса с данными текущего пользователя и его организации.
type Me struct {
	User         User
	Organization *service.Org
}

// AuthStore описывает минимальный набор методов хранилища для аутентификации.
type AuthStore interface {
	GetUserByEmail(ctx context.Context, email string) (repository.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (repository.User, error)
	CreateUser(ctx context.Context, arg repository.CreateUserParams) (repository.CreateUserRow, error)
	CreateOrganization(ctx context.Context, arg repository.CreateOrganizationParams) (repository.Organization, error)
	GetOrganizationByID(ctx context.Context, id uuid.UUID) (repository.Organization, error)
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error
	CreateSession(ctx context.Context, arg repository.CreateSessionParams) (repository.CreateSessionRow, error)
	CreateAuditLog(ctx context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash []byte) (repository.GetSessionByTokenHashRow, error)
	TouchSession(ctx context.Context, tokenHash []byte) error
	DeleteSessionByTokenHash(ctx context.Context, tokenHash []byte) error
}

// AuthTx описывает транзакционный контекст для работы с репозиторием.
type AuthTx interface {
	InTx(ctx context.Context, fn func(q repository.Querier) error) error
}

// AuthLimits — лимиты попыток входа и регистрации (скользящее окно).
type AuthLimits struct {
	LoginPerEmail  int // попыток на один email за LoginWindow; успешный вход сбрасывает счётчик
	LoginPerIP     int // попыток с одного IP за LoginWindow
	LoginWindow    time.Duration
	RegisterPerIP  int // регистраций с одного IP за RegisterWindow
	RegisterWindow time.Duration
}

// DefaultAuthLimits — лимиты из плана BE-03.
var DefaultAuthLimits = AuthLimits{
	LoginPerEmail:  10,
	LoginPerIP:     50,
	LoginWindow:    15 * time.Minute,
	RegisterPerIP:  10,
	RegisterWindow: time.Hour,
}

// Auth содержит зависимости аутентификации и TTL сессии.
type Auth struct {
	store        AuthStore
	tx           AuthTx
	sessionTTL   time.Duration
	limiter      service.RateLimiter
	limits       AuthLimits
	fakePassword string
}

// NewAuth создаёт сервис аутентификации.
func NewAuth(store AuthStore, tx AuthTx, sessionTTL time.Duration, limiter service.RateLimiter, limits AuthLimits) *Auth {
	if limiter == nil {
		panic("service: nil rate limiter")
	}
	hash, err := HashPassword(context.Background(), "auth-dummy-guard")
	if err != nil {
		panic(err)
	}
	return &Auth{store: store, tx: tx, sessionTTL: sessionTTL, limiter: limiter, limits: limits, fakePassword: hash}
}

// allow проверяет лимит до любой работы с паролем: иначе поток запросов занимает
// слоты argon2id (password.go) и ставит в очередь логины настоящих пользователей.
func (a *Auth) allow(key string, limit int, window time.Duration) error {
	if retryAfter, ok := a.limiter.Allow(key, limit, window); !ok {
		return apperr.RateLimitedAfter(retryAfter)
	}
	return nil
}

// Register регистрирует нового пользователя и сразу создаёт активную сессию.
func (a *Auth) Register(ctx context.Context, in RegisterInput, meta service.ClientMeta) (*Session, error) {
	if err := a.allow("register:ip:"+meta.IPKey(), a.limits.RegisterPerIP, a.limits.RegisterWindow); err != nil {
		return nil, err
	}
	email := normalizeEmail(in.Email)
	if err := ValidatePassword(in.Password); err != nil {
		return nil, err
	}

	role := strings.TrimSpace(strings.ToLower(in.Role))
	if role != "borrower" && role != "lender" {
		return nil, apperr.Validation(
			"Некорректная роль пользователя",
			apperr.FieldError{Field: "role", Message: "допустимы borrower или lender"},
		)
	}

	countryCode := strings.TrimSpace(strings.ToUpper(in.Org.CountryCode))
	if role == "borrower" && countryCode != "RU" {
		return nil, apperr.Validation(
			"Некорректная страна организации",
			apperr.FieldError{Field: "organization.country_code", Message: "для borrower требуется RU"},
		)
	}
	if role == "lender" && countryCode == "RU" {
		return nil, apperr.Validation(
			"Некорректная страна организации",
			apperr.FieldError{Field: "organization.country_code", Message: "для lender страна не должна быть RU"},
		)
	}

	orgType := repository.OrganizationTypeBorrower
	if role == "lender" {
		orgType = repository.OrganizationTypeLender
	}

	passwordHash, err := HashPassword(ctx, in.Password)
	if err != nil {
		return nil, err
	}

	var created *Session
	err = a.tx.InTx(ctx, func(q repository.Querier) error {
		org, err := q.CreateOrganization(ctx, repository.CreateOrganizationParams{
			Type:               orgType,
			Name:               strings.TrimSpace(in.Org.Name),
			CountryCode:        countryCode,
			RegistrationNumber: strings.TrimSpace(in.Org.RegistrationNumber),
			Lei:                pgconv.Text(in.Org.LEI),
			LegalAddress:       strings.TrimSpace(in.Org.LegalAddress),
		})
		if err != nil {
			return mapConflict(err)
		}

		userRow, err := q.CreateUser(ctx, repository.CreateUserParams{
			Email:          email,
			PasswordHash:   passwordHash,
			Role:           repository.UserRole(role),
			OrganizationID: uuidToPgUUID(org.ID),
		})
		if err != nil {
			return mapConflict(err)
		}

		token, csrfToken, expiresAt, err := a.newSession()
		if err != nil {
			return err
		}

		_, err = q.CreateSession(ctx, repository.CreateSessionParams{
			TokenHash: hashToken(token),
			UserID:    userRow.ID,
			ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
			Ip:        meta.IP,
			UserAgent: pgtype.Text{String: meta.UserAgent, Valid: meta.UserAgent != ""},
		})
		if err != nil {
			return err
		}

		// Аудит — в транзакции регистрации: его сбой откатывает пользователя и организацию.
		if err := service.WriteAudit(ctx, q, meta, userAudit(AuditAuthRegister, userRow.ID, map[string]any{
			"organization_id": org.ID.String(),
		})); err != nil {
			return err
		}

		created = &Session{
			Token:     token,
			CSRFToken: csrfToken,
			ExpiresAt: expiresAt,
			Me: &Me{
				User: userFromRepositoryUser(repository.User{
					ID:             userRow.ID,
					Email:          userRow.Email,
					Role:           userRow.Role,
					Status:         userRow.Status,
					OrganizationID: userRow.OrganizationID,
					CreatedAt:      userRow.CreatedAt,
				}),
				Organization: service.OrgFromRepository(org),
			},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Login выполняет вход по email и паролю с защитой от тайминг-атак.
//
// Каждая попытка учитывается в лимитах на IP и на email; успешный вход сбрасывает
// счётчик email, так что по email фактически считаются неудачные попытки.
func (a *Auth) Login(ctx context.Context, email, password string, meta service.ClientMeta) (*Session, error) {
	normalizedEmail := normalizeEmail(email)
	if err := a.allow("login:ip:"+meta.IPKey(), a.limits.LoginPerIP, a.limits.LoginWindow); err != nil {
		return nil, err
	}
	emailKey := "login:email:" + normalizedEmail
	if err := a.allow(emailKey, a.limits.LoginPerEmail, a.limits.LoginWindow); err != nil {
		return nil, err
	}

	userRow, err := a.store.GetUserByEmail(ctx, normalizedEmail)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if _, verifyErr := VerifyPassword(ctx, password, a.fakePassword); verifyErr != nil {
			return nil, verifyErr
		}
		a.auditFailure(ctx, nil, "unknown_email", meta)
		return nil, apperr.Unauthorized("Неверный email или пароль")
	}

	ok, verifyErr := VerifyPassword(ctx, password, userRow.PasswordHash)
	if verifyErr != nil {
		return nil, verifyErr
	}
	if !ok {
		userID := userRow.ID
		a.auditFailure(ctx, &userID, "bad_password", meta)
		return nil, apperr.Unauthorized("Неверный email или пароль")
	}
	if userRow.Status == repository.UserStatusBlocked {
		userID := userRow.ID
		a.auditFailure(ctx, &userID, "blocked", meta)
		return nil, apperr.Forbidden("Аккаунт заблокирован")
	}

	token, csrfToken, expiresAt, err := a.newSession()
	if err != nil {
		return nil, err
	}

	// last_login_at, сессия и аудит входа — одна транзакция: если что-то упало,
	// клиент получает ошибку и в БД не остаётся живой сессии.
	userID := userRow.ID
	err = a.tx.InTx(ctx, func(q repository.Querier) error {
		if err := q.UpdateLastLogin(ctx, userID); err != nil {
			return err
		}
		if _, err := q.CreateSession(ctx, repository.CreateSessionParams{
			TokenHash: hashToken(token),
			UserID:    userID,
			ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
			Ip:        meta.IP,
			UserAgent: pgtype.Text{String: meta.UserAgent, Valid: meta.UserAgent != ""},
		}); err != nil {
			return err
		}
		return service.WriteAudit(ctx, q, meta, userAudit(AuditAuthLogin, userID, nil))
	})
	if err != nil {
		return nil, err
	}
	a.limiter.Reset(emailKey)

	me := &Me{User: userFromRepositoryUser(userRow)}
	if userRow.OrganizationID.Valid {
		orgID := userRow.OrganizationID.Bytes
		orgRow, orgErr := a.store.GetOrganizationByID(ctx, orgID)
		if orgErr != nil {
			return nil, pgerr.Map(orgErr, "Организация не найдена")
		}
		me.Organization = service.OrgFromRepository(orgRow)
	}

	return &Session{Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt, Me: me}, nil
}

// auditFailure пишет auth.login_failed best-effort: транзакции нет, ошибка записи
// только логируется и не меняет ответ клиенту. Email при unknown_email не пишется —
// это ПДн чужого человека (DECISIONS п. 16).
func (a *Auth) auditFailure(ctx context.Context, userID *uuid.UUID, reason string, meta service.ClientMeta) {
	event := service.AuditEvent{Action: AuditAuthLoginFailed, Meta: map[string]any{"reason": reason}}
	if userID != nil {
		event = userAudit(AuditAuthLoginFailed, *userID, event.Meta)
	}
	if err := service.WriteAudit(ctx, a.store, meta, event); err != nil {
		slog.WarnContext(ctx, "audit write failed", "action", AuditAuthLoginFailed, "err", err)
	}
}

// userAudit — событие о пользователе: он же и автор, и сущность.
func userAudit(action string, userID uuid.UUID, fields map[string]any) service.AuditEvent {
	return service.AuditEvent{
		Action:     action,
		UserID:     &userID,
		EntityType: repository.EntityTypeUser,
		EntityID:   userID.String(),
		Meta:       fields,
	}
}

// Authenticate возвращает Principal по токену сессии.
func (a *Auth) Authenticate(ctx context.Context, token string) (*service.Principal, error) {
	if token == "" {
		return nil, apperr.Unauthorized("Сессия недействительна")
	}
	row, err := a.store.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.Unauthorized("Сессия недействительна")
		}
		return nil, err
	}
	if err := a.store.TouchSession(ctx, hashToken(token)); err != nil {
		slog.WarnContext(ctx, "touch session failed", "err", err)
	}

	var orgID *uuid.UUID
	if row.OrganizationID.Valid {
		v := uuid.UUID(row.OrganizationID.Bytes)
		orgID = &v
	}
	var kycStatus string // у admin организации нет: LEFT JOIN даёт NULL
	if row.KycStatus.Valid {
		kycStatus = string(row.KycStatus.KycStatus)
	}
	return &service.Principal{UserID: row.UserID, OrgID: orgID, Role: string(row.Role), KycStatus: kycStatus}, nil
}

// Logout удаляет сессию по токену.
func (a *Auth) Logout(ctx context.Context, token string) error {
	return a.store.DeleteSessionByTokenHash(ctx, hashToken(token))
}

// Me загружает доменные данные текущего пользователя и его организации.
func (a *Auth) Me(ctx context.Context, p service.Principal) (*Me, error) {
	row, err := a.store.GetUserByID(ctx, p.UserID)
	if err != nil {
		return nil, pgerr.Map(err, "Пользователь не найден")
	}
	me := &Me{User: userFromRepositoryUser(row)}
	if p.OrgID != nil {
		orgRow, err := a.store.GetOrganizationByID(ctx, *p.OrgID)
		if err != nil {
			return nil, pgerr.Map(err, "Организация не найдена")
		}
		me.Organization = service.OrgFromRepository(orgRow)
	}
	return me, nil
}

func (a *Auth) newSession() (string, string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	csrfToken, err := randomToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	return token, csrfToken, time.Now().Add(a.sessionTTL), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func uuidToPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// Уникальные ограничения из migrations/00001_init.sql, которые может нарушить регистрация.
const (
	constraintUserEmail = "users_email_key"
	constraintOrgRegnum = "organizations_regnum_uq"
	constraintOrgLEI    = "organizations_lei_uq"
)

// mapConflict переводит нарушение уникальности при регистрации в 409 с текстом,
// по которому пользователь поймёт, что именно занято. Остальное — через pgerr.Map.
func mapConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case constraintUserEmail:
			return apperr.Wrap(apperr.CodeConflict, "Пользователь с таким email уже существует", err)
		case constraintOrgRegnum, constraintOrgLEI:
			return apperr.Wrap(apperr.CodeConflict, "Организация с таким номером уже зарегистрирована", err)
		}
	}
	if _, ok := apperr.As(err); ok {
		return err
	}
	return pgerr.Map(err, "Запись не найдена")
}

func userFromRepositoryUser(u repository.User) User {
	meUser := User{ID: u.ID, Email: u.Email, Role: string(u.Role), Status: string(u.Status), CreatedAt: u.CreatedAt.Time}
	if u.OrganizationID.Valid {
		v := uuid.UUID(u.OrganizationID.Bytes)
		meUser.OrganizationID = &v
	}
	if u.LastLoginAt.Valid {
		v := u.LastLoginAt.Time
		meUser.LastLoginAt = &v
	}
	return meUser
}

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ClientMeta содержит сетевую информацию клиента для создания сессии.
type ClientMeta struct {
	IP        *netip.Addr
	UserAgent string
}

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

// Org — доменная модель организации для ответа `Me`.
type Org struct {
	ID                 uuid.UUID
	Type               string
	Name               string
	CountryCode        string
	RegistrationNumber string
	LEI                *string
	LegalAddress       string
	KycStatus          string
	CreatedAt          time.Time
	VerifiedAt         *time.Time
	KycRejectReason    *string
}

// Me — ответ сервиса с данными текущего пользователя и его организации.
type Me struct {
	User         User
	Organization *Org
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
	GetSessionByTokenHash(ctx context.Context, tokenHash []byte) (repository.GetSessionByTokenHashRow, error)
	TouchSession(ctx context.Context, tokenHash []byte) error
	DeleteSessionByTokenHash(ctx context.Context, tokenHash []byte) error
}

// AuthTx описывает транзакционный контекст для работы с репозиторием.
type AuthTx interface {
	InTx(ctx context.Context, fn func(q repository.Querier) error) error
}

// Auth содержит зависимости аутентификации и TTL сессии.
type Auth struct {
	store        AuthStore
	tx           AuthTx
	sessionTTL   time.Duration
	fakePassword string
}

// NewAuth создаёт сервис аутентификации.
func NewAuth(store AuthStore, tx AuthTx, sessionTTL time.Duration) *Auth {
	hash, err := HashPassword(context.Background(), "auth-dummy-guard")
	if err != nil {
		panic(err)
	}
	return &Auth{store: store, tx: tx, sessionTTL: sessionTTL, fakePassword: hash}
}

// Register регистрирует нового пользователя и сразу создаёт активную сессию.
func (a *Auth) Register(ctx context.Context, in RegisterInput, meta ClientMeta) (*Session, error) {
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
			Lei:                textPtr(in.Org.LEI),
			LegalAddress:       strings.TrimSpace(in.Org.LegalAddress),
		})
		if err != nil {
			return mapConflict(err, "Организация уже существует")
		}

		userRow, err := q.CreateUser(ctx, repository.CreateUserParams{
			Email:          email,
			PasswordHash:   passwordHash,
			Role:           repository.UserRole(role),
			OrganizationID: uuidToPgUUID(org.ID),
		})
		if err != nil {
			return mapConflict(err, "Пользователь уже существует")
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
				Organization: organizationFromRepository(org),
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
func (a *Auth) Login(ctx context.Context, email, password string, meta ClientMeta) (*Session, error) {
	normalizedEmail := normalizeEmail(email)
	userRow, err := a.store.GetUserByEmail(ctx, normalizedEmail)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if _, verifyErr := VerifyPassword(ctx, password, a.fakePassword); verifyErr != nil {
			return nil, verifyErr
		}
		return nil, apperr.Unauthorized("Неверный email или пароль")
	}

	ok, verifyErr := VerifyPassword(ctx, password, userRow.PasswordHash)
	if verifyErr != nil {
		return nil, verifyErr
	}
	if !ok {
		return nil, apperr.Unauthorized("Неверный email или пароль")
	}
	if userRow.Status == repository.UserStatusBlocked {
		return nil, apperr.Forbidden("Аккаунт заблокирован")
	}

	if err := a.store.UpdateLastLogin(ctx, userRow.ID); err != nil {
		return nil, err
	}

	token, csrfToken, expiresAt, err := a.newSession()
	if err != nil {
		return nil, err
	}

	_, err = a.store.CreateSession(ctx, repository.CreateSessionParams{
		TokenHash: hashToken(token),
		UserID:    userRow.ID,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		Ip:        meta.IP,
		UserAgent: pgtype.Text{String: meta.UserAgent, Valid: meta.UserAgent != ""},
	})
	if err != nil {
		return nil, err
	}

	me := &Me{User: userFromRepositoryUser(userRow)}
	if userRow.OrganizationID.Valid {
		orgID := userRow.OrganizationID.Bytes
		orgRow, orgErr := a.store.GetOrganizationByID(ctx, orgID)
		if orgErr != nil {
			return nil, pgerr.Map(orgErr, "Организация не найдена")
		}
		me.Organization = organizationFromRepository(orgRow)
	}

	return &Session{Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt, Me: me}, nil
}

// Authenticate возвращает Principal по токену сессии.
func (a *Auth) Authenticate(ctx context.Context, token string) (*Principal, error) {
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
		slog.Warn("touch session failed", "err", err)
	}

	var orgID *uuid.UUID
	if row.OrganizationID.Valid {
		v := uuid.UUID(row.OrganizationID.Bytes)
		orgID = &v
	}
	return &Principal{UserID: row.UserID, OrgID: orgID, Role: string(row.Role)}, nil
}

// Logout удаляет сессию по токену.
func (a *Auth) Logout(ctx context.Context, token string) error {
	return a.store.DeleteSessionByTokenHash(ctx, hashToken(token))
}

// Me загружает доменные данные текущего пользователя и его организации.
func (a *Auth) Me(ctx context.Context, p Principal) (*Me, error) {
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
		me.Organization = organizationFromRepository(orgRow)
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

func textPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func uuidToPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func mapConflict(err error, fallback string) error {
	if err == nil {
		return nil
	}
	if e, ok := apperr.As(err); ok {
		return e
	}
	mapped := pgerr.Map(err, fallback)
	if mapped == nil {
		return err
	}
	return mapped
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

func organizationFromRepository(o repository.Organization) *Org {
	org := Org{
		ID:                 o.ID,
		Type:               string(o.Type),
		Name:               o.Name,
		CountryCode:        o.CountryCode,
		RegistrationNumber: o.RegistrationNumber,
		LegalAddress:       o.LegalAddress,
		KycStatus:          string(o.KycStatus),
		CreatedAt:          o.CreatedAt.Time,
	}
	if o.Lei.Valid {
		v := o.Lei.String
		org.LEI = &v
	}
	if o.VerifiedAt.Valid {
		v := o.VerifiedAt.Time
		org.VerifiedAt = &v
	}
	if o.KycRejectReason.Valid {
		v := o.KycRejectReason.String
		org.KycRejectReason = &v
	}
	return &org
}

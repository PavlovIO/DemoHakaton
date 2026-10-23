package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/authapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Register регистрирует пользователя и устанавливает cookies сессии.
func (h *Handler) Register(ctx context.Context, request authapi.RegisterRequestObject) (authapi.RegisterResponseObject, error) {
	if request.Body == nil {
		return nil, apperr.Validation("Некорректное тело запроса")
	}
	input := service.RegisterInput{
		Email:    string(request.Body.Email),
		Password: request.Body.Password,
		Role:     string(request.Body.Role),
		Org: service.RegisterOrganizationInput{
			Name:               request.Body.Organization.Name,
			CountryCode:        request.Body.Organization.CountryCode,
			RegistrationNumber: request.Body.Organization.RegistrationNumber,
			LEI:                request.Body.Organization.LEI,
			LegalAddress:       request.Body.Organization.LegalAddress,
		},
	}
	session, err := h.auth.Register(ctx, input, middleware.ClientMetaFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return authRegisterResponse{
		body:         meToAPI(session.Me),
		token:        session.Token,
		csrfToken:    session.CSRFToken,
		expiresAt:    session.ExpiresAt,
		cookieSecure: h.cookieSecure,
	}, nil
}

// Login выполняет вход и устанавливает cookies сессии.
func (h *Handler) Login(ctx context.Context, request authapi.LoginRequestObject) (authapi.LoginResponseObject, error) {
	if request.Body == nil {
		return nil, apperr.Validation("Некорректное тело запроса")
	}
	session, err := h.auth.Login(
		ctx,
		string(request.Body.Email),
		request.Body.Password,
		middleware.ClientMetaFromContext(ctx),
	)
	if err != nil {
		return nil, err
	}
	return authLoginResponse{
		body:         meToAPI(session.Me),
		token:        session.Token,
		csrfToken:    session.CSRFToken,
		expiresAt:    session.ExpiresAt,
		cookieSecure: h.cookieSecure,
	}, nil
}

// Logout завершает сессию и очищает обе cookies.
func (h *Handler) Logout(ctx context.Context, _ authapi.LogoutRequestObject) (authapi.LogoutResponseObject, error) {
	token, ok := middleware.SessionTokenFromContext(ctx)
	if !ok || token == "" {
		return nil, apperr.Unauthorized("Сессия недействительна")
	}
	if err := h.auth.Logout(ctx, token); err != nil {
		return nil, err
	}
	return authLogoutResponse{cookieSecure: h.cookieSecure}, nil
}

// GetMe возвращает профиль пользователя из principal контекста.
func (h *Handler) GetMe(ctx context.Context, _ authapi.GetMeRequestObject) (authapi.GetMeResponseObject, error) {
	principal, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	me, err := h.auth.Me(ctx, principal)
	if err != nil {
		return nil, err
	}
	return authapi.GetMe200JSONResponse(meToAPI(me)), nil
}

type authRegisterResponse struct {
	body         api.Me
	token        string
	csrfToken    string
	expiresAt    time.Time
	cookieSecure bool
}

func (response authRegisterResponse) VisitRegisterResponse(w http.ResponseWriter) error {
	return writeAuthJSON(w, http.StatusCreated, response.body, response.sessionCookies())
}

func (response authRegisterResponse) sessionCookies() []*http.Cookie {
	return sessionCookies(response.token, response.csrfToken, response.expiresAt, response.cookieSecure)
}

type authLoginResponse struct {
	body         api.Me
	token        string
	csrfToken    string
	expiresAt    time.Time
	cookieSecure bool
}

func (response authLoginResponse) VisitLoginResponse(w http.ResponseWriter) error {
	return writeAuthJSON(w, http.StatusOK, response.body, sessionCookies(response.token, response.csrfToken, response.expiresAt, response.cookieSecure))
}

type authLogoutResponse struct {
	cookieSecure bool
}

func (response authLogoutResponse) VisitLogoutResponse(w http.ResponseWriter) error {
	w.Header().Add("Set-Cookie", expiredCookie(middleware.SessionCookieName, true, response.cookieSecure).String())
	w.Header().Add("Set-Cookie", expiredCookie(middleware.CSRFCookieName, false, response.cookieSecure).String())
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func sessionCookies(token, csrfToken string, expiresAt time.Time, secure bool) []*http.Cookie {
	return []*http.Cookie{
		{Name: middleware.SessionCookieName, Value: token, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, Path: "/", Expires: expiresAt},
		{Name: middleware.CSRFCookieName, Value: csrfToken, HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode, Path: "/", Expires: expiresAt},
	}
}

func expiredCookie(name string, httpOnly, secure bool) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: "", HttpOnly: httpOnly, Secure: secure,
		SameSite: http.SameSiteLaxMode, Path: "/", Expires: time.Unix(1, 0), MaxAge: -1,
	}
}

func writeAuthJSON(w http.ResponseWriter, status int, body api.Me, cookies []*http.Cookie) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		w.Header().Add("Set-Cookie", cookie.String())
	}
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}

func meToAPI(me *service.Me) api.Me {
	user := api.User{
		ID:        me.User.ID,
		Email:     openapi_types.Email(me.User.Email),
		Role:      api.Role(me.User.Role),
		Status:    api.UserStatus(me.User.Status),
		CreatedAt: me.User.CreatedAt,
	}
	if me.User.OrganizationID != nil {
		orgID := *me.User.OrganizationID
		user.OrganizationID = &orgID
	}

	var organization *api.Organization
	if me.Organization != nil {
		organization = &api.Organization{
			ID:                 me.Organization.ID,
			Type:               api.OrganizationType(me.Organization.Type),
			Name:               me.Organization.Name,
			CountryCode:        me.Organization.CountryCode,
			RegistrationNumber: me.Organization.RegistrationNumber,
			LEI:                me.Organization.LEI,
			LegalAddress:       me.Organization.LegalAddress,
			KYCStatus:          api.KYCStatus(me.Organization.KycStatus),
			CreatedAt:          me.Organization.CreatedAt,
			VerifiedAt:         me.Organization.VerifiedAt,
			KYCRejectReason:    me.Organization.KycRejectReason,
			Beneficiaries:      []api.Beneficiary{},
		}
	}
	return api.Me{User: user, Organization: organization}
}

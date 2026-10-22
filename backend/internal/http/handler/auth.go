package handler

// Операции тега auth (authapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/authapi"
)

func (h *Handler) Login(context.Context, authapi.LoginRequestObject) (authapi.LoginResponseObject, error) {
	return nil, apperr.NotImplemented("login")
}

func (h *Handler) Logout(context.Context, authapi.LogoutRequestObject) (authapi.LogoutResponseObject, error) {
	return nil, apperr.NotImplemented("logout")
}

func (h *Handler) GetMe(context.Context, authapi.GetMeRequestObject) (authapi.GetMeResponseObject, error) {
	return nil, apperr.NotImplemented("getMe")
}

func (h *Handler) Register(context.Context, authapi.RegisterRequestObject) (authapi.RegisterResponseObject, error) {
	return nil, apperr.NotImplemented("register")
}

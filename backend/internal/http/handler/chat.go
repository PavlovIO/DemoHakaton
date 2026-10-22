package handler

// Операции тега chat (chatapi.StrictServerInterface).
// Ещё не реализованные отвечают 501 NOT_IMPLEMENTED в формате Error. Реализуя операцию,
// замените тело заглушки (образец — dictionaries.go + dictionaries_test.go).

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/chatapi"
)

func (h *Handler) ListMessages(context.Context, chatapi.ListMessagesRequestObject) (chatapi.ListMessagesResponseObject, error) {
	return nil, apperr.NotImplemented("listMessages")
}

func (h *Handler) SendMessage(context.Context, chatapi.SendMessageRequestObject) (chatapi.SendMessageResponseObject, error) {
	return nil, apperr.NotImplemented("sendMessage")
}

package handler

// Операции тега chat (chatapi.StrictServerInterface): перевод api ⇄ service.
// Доступ (стороны оффера и admin), статус оффера и лимит отправки проверяет service.Chat.

import (
	"context"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/chatapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// defaultMessagesLimit — default параметра limit у listMessages (api/paths/chat.yaml):
// кодоген default не подставляет, nil приходит сюда. Диапазон 1–200 проверил валидатор.
const defaultMessagesLimit = 50

func (h *Handler) ListMessages(ctx context.Context, req chatapi.ListMessagesRequestObject) (chatapi.ListMessagesResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var afterID int64
	if req.Params.AfterID != nil {
		afterID = *req.Params.AfterID
	}
	limit := defaultMessagesLimit
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	messages, err := h.chat.List(ctx, p, req.OfferID, afterID, int32(limit)) //nolint:gosec // limit ≤ 200 по спеке
	if err != nil {
		return nil, err
	}
	out := make(chatapi.ListMessages200JSONResponse, len(messages))
	for i, m := range messages {
		out[i] = toAPIMessage(m)
	}
	return out, nil
}

func (h *Handler) SendMessage(ctx context.Context, req chatapi.SendMessageRequestObject) (chatapi.SendMessageResponseObject, error) {
	p, err := currentPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.chat.Send(ctx, p, req.OfferID, req.Body.Text)
	if err != nil {
		return nil, err
	}
	return chatapi.SendMessage201JSONResponse(toAPIMessage(m)), nil
}

func toAPIMessage(m service.Message) api.Message {
	isMine := m.IsMine
	return api.Message{
		ID:                 m.ID,
		OfferID:            m.OfferID,
		AuthorUserID:       m.AuthorUserID,
		AuthorRole:         api.Role(m.AuthorRole),
		AuthorOrganization: api.OrganizationShort{ID: m.AuthorOrg.ID, Name: m.AuthorOrg.Name, CountryCode: m.AuthorOrg.CountryCode},
		Text:               m.Text,
		IsMine:             &isMine,
		CreatedAt:          m.CreatedAt,
	}
}

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// Лимит отправки — 20 сообщений в минуту на пользователя (DECISIONS п. 15): с запасом для
// живой переписки. Префикс ключа отделяет чат от лимитов auth в общем MemoryRateLimiter.
const (
	chatSendKeyPrefix = "chat:send:"
	chatSendLimit     = 20
	chatSendWindow    = time.Minute
)

// Message — сообщение чата по офферу, как его видит конкретный пользователь.
type Message struct {
	ID           int64
	OfferID      uuid.UUID
	AuthorUserID uuid.UUID
	AuthorRole   repository.UserRole
	AuthorOrg    OrgShort
	Text         string
	IsMine       bool
	CreatedAt    time.Time
}

// ChatStore — запросы вне транзакции (чтение чата).
type ChatStore interface {
	GetOfferVisible(ctx context.Context, arg repository.GetOfferVisibleParams) (repository.GetOfferVisibleRow, error)
	ListMessagesVisible(ctx context.Context, arg repository.ListMessagesVisibleParams) ([]repository.ListMessagesVisibleRow, error)
	MarkMessagesRead(ctx context.Context, arg repository.MarkMessagesReadParams) error
}

// ChatDeps — зависимости сервиса чата.
type ChatDeps struct {
	Store   ChatStore
	Tx      TxRunner
	Limiter RateLimiter
}

// Chat — чат сторон оффера: заёмщик ↔ кредитор, admin только читает.
type Chat struct {
	store    ChatStore
	tx       TxRunner
	notifier Notifier
	limiter  RateLimiter
}

// NewChat создаёт сервис чата.
func NewChat(d ChatDeps) *Chat {
	return &Chat{store: d.Store, tx: d.Tx, limiter: d.Limiter}
}

// List — сообщения чата оффера с id > afterID по возрастанию, не больше limit; afterID = 0 —
// самые старые (polling, DECISIONS п. 15). Невидимый оффер → 404.
//
// GET сдвигает указатель «прочитано» — так требует контракт (listMessages). Это допустимо:
// меняется только указатель самого читателя, он монотонный (GREATEST) и идемпотентный.
func (s *Chat) List(ctx context.Context, p Principal, offerID uuid.UUID, afterID int64, limit int32) ([]Message, error) {
	if _, err := s.store.GetOfferVisible(ctx, repository.GetOfferVisibleParams{
		ID: offerID, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(), ViewerUser: p.UserID,
	}); err != nil {
		return nil, pgerr.Map(err, offerNotFound)
	}
	rows, err := s.store.ListMessagesVisible(ctx, repository.ListMessagesVisibleParams{
		OfferID: offerID, AfterID: afterID, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(), MaxRows: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	out := make([]Message, len(rows))
	for i, row := range rows {
		out[i] = messageFromRow(p, row)
	}
	// Admin указатель не пишет: он не сторона чата, счётчика unread_messages у него нет.
	if len(out) > 0 && p.ViewerRole() != repository.UserRoleAdmin {
		// MarkMessagesRead не проверяет доступ: оффер уже прошёл GetOfferVisible выше.
		if err := s.store.MarkMessagesRead(ctx, repository.MarkMessagesReadParams{
			OfferID: offerID, UserID: p.UserID, LastReadMessageID: out[len(out)-1].ID,
		}); err != nil {
			return nil, fmt.Errorf("mark messages read: %w", err)
		}
	}
	return out, nil
}

// Send пишет сообщение стороны оффера и уведомляет другую сторону. Текст хранится как есть
// (без trim и санитизации): экранирует фронт, длину 1–4000 проверили валидатор и CHECK в БД.
//
// Лимит проверяется после 404 и 409: иначе повторы отправки в отозванный оффер исчерпали бы
// лимит и дали 429 во всех чатах пользователя. Allow атомарно учитывает попытку, поэтому
// гонки между проверкой и записью попытки нет.
func (s *Chat) Send(ctx context.Context, p Principal, offerID uuid.UUID, text string) (Message, error) {
	// Admin отсекает ещё middleware по x-roles; проверка здесь — на случай вызова сервиса в обход HTTP.
	if err := p.RequireRole(repository.UserRoleBorrower, repository.UserRoleLender); err != nil {
		return Message{}, err
	}
	var out Message
	err := s.tx.InTx(ctx, func(q repository.Querier) error {
		// CreateMessage не проверяет доступ: оффер обязан пройти GetOfferVisible раньше в той же транзакции.
		offer, err := q.GetOfferVisible(ctx, repository.GetOfferVisibleParams{
			ID: offerID, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(), ViewerUser: p.UserID,
		})
		if err != nil {
			return pgerr.Map(err, offerNotFound)
		}
		if err := requireChatOpen(offer.Offer.Status); err != nil {
			return err
		}
		if retryAfter, ok := s.limiter.Allow(chatSendKeyPrefix+p.UserID.String(), chatSendLimit, chatSendWindow); !ok {
			return apperr.RateLimitedAfter(retryAfter)
		}
		// Без блокировки polling по id > after_id может пропустить сообщение (см. LockOfferChat).
		if err := q.LockOfferChat(ctx, offerID); err != nil {
			return fmt.Errorf("lock offer chat: %w", err)
		}
		msg, err := q.CreateMessage(ctx, repository.CreateMessageParams{OfferID: offerID, AuthorUserID: p.UserID, Text: text})
		if err != nil {
			return fmt.Errorf("create message: %w", err)
		}
		// Одно уведомление на каждое сообщение, без склейки (DECISIONS п. 14).
		if err := s.notifier.NotifyOrganization(ctx, q, otherParty(p, offer), offerNotification(
			repository.NotificationTypeMessageReceived, "Новое сообщение по офферу", offerID,
		)); err != nil {
			return err
		}
		// Перечитываем тем же запросом, что и GET: одна конвертация с автором и его компанией.
		rows, err := q.ListMessagesVisible(ctx, repository.ListMessagesVisibleParams{
			OfferID: offerID, AfterID: msg.ID - 1, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(), MaxRows: 1,
		})
		if err != nil {
			return fmt.Errorf("reread message %d: %w", msg.ID, err)
		}
		if len(rows) == 0 {
			return fmt.Errorf("reread message %d: not visible", msg.ID)
		}
		out = messageFromRow(p, rows[0])
		return nil
	})
	return out, err
}

// requireChatOpen — писать можно, пока оффер sent (торг) или accepted (исполнение сделки).
// Это не переход статуса, но код тот же, что у действия с неподходящим статусом: 409
// INVALID_STATUS_TRANSITION (DECISIONS п. 6). Оффер не блокируется: сообщение, вставшее
// в один момент с отзывом оффера, безвредно.
func requireChatOpen(status repository.OfferStatus) error {
	if status == repository.OfferStatusSent || status == repository.OfferStatusAccepted {
		return nil
	}
	return apperr.New(apperr.CodeInvalidTransition, "Чат закрыт: оффер в статусе "+string(status))
}

// otherParty — организация, которой адресовано сообщение p: кредитору пишет заёмщик и наоборот.
func otherParty(p Principal, offer repository.GetOfferVisibleRow) uuid.UUID {
	if p.ViewerRole() == repository.UserRoleLender {
		return offer.BorrowerOrganizationID
	}
	return offer.Offer.LenderOrganizationID
}

func messageFromRow(p Principal, row repository.ListMessagesVisibleRow) Message {
	m := row.Message
	return Message{
		ID:           m.ID,
		OfferID:      m.OfferID,
		AuthorUserID: m.AuthorUserID,
		AuthorRole:   row.AuthorRole,
		AuthorOrg:    OrgShort{ID: row.AuthorOrgID, Name: row.AuthorOrgName, CountryCode: row.AuthorOrgCountryCode},
		Text:         m.Text,
		IsMine:       m.AuthorUserID == p.UserID,
		CreatedAt:    m.CreatedAt.Time,
	}
}

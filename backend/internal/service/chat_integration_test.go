package service_test

// Интеграционный тест чата на настоящем Postgres: видимость ListMessagesVisible с JOIN оффера
// и заявки, автор с компанией, message_reads, уведомление message_received.
// Сервис открывает транзакции через tx.Begin (savepoint) — всё откатывается в Cleanup.

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func TestChatIntegration(t *testing.T) {
	ctx := context.Background()
	tx := beginTestTx(t)
	q := repository.New(tx)
	txm := service.NewTxManager(tx)
	chat := service.NewChat(service.ChatDeps{Store: q, Tx: txm, Limiter: service.NewMemoryRateLimiter()})
	offers := service.NewOffers(service.OffersDeps{Store: q, Tx: txm})

	b1 := insertDealsOrg(ctx, t, tx, repository.UserRoleBorrower, "Chat B1")
	b2 := insertDealsOrg(ctx, t, tx, repository.UserRoleBorrower, "Chat B2")
	l1 := insertDealsOrg(ctx, t, tx, repository.UserRoleLender, "Chat L1")
	l2 := insertDealsOrg(ctx, t, tx, repository.UserRoleLender, "Chat L2")
	admin := service.Principal{UserID: insertTestUser(ctx, t, tx, repository.UserRoleAdmin, nil), Role: "admin"}
	appID := insertDealsApp(ctx, t, tx, b1)
	offerL1 := insertDealsOffer(ctx, t, tx, appID, l1, nil)
	insertDealsOffer(ctx, t, tx, appID, l2, nil) // второй оффер L2: свой у L2 есть, но чат L1 ему чужой

	send := func(t *testing.T, p service.Principal, offerID uuid.UUID, text string) service.Message {
		t.Helper()
		m, err := chat.Send(ctx, p, offerID, text)
		if err != nil {
			t.Fatalf("send %q: %v", text, err)
		}
		return m
	}
	countReads := func(t *testing.T, offerID, userID uuid.UUID) int {
		t.Helper()
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM message_reads WHERE offer_id = $1 AND user_id = $2`,
			offerID, userID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("заёмщик пишет, L1 читает — автор, is_mine, указатель, unread = 0", func(t *testing.T) {
		sent := send(t, b1, offerL1, "Здравствуйте")
		if !sent.IsMine || sent.AuthorRole != repository.UserRoleBorrower || sent.AuthorOrg.Name != "Deals Chat B1" || sent.CreatedAt.IsZero() {
			t.Fatalf("sent = %+v", sent)
		}
		got, err := chat.List(ctx, l1, offerL1, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != sent.ID || got[0].IsMine || got[0].AuthorRole != repository.UserRoleBorrower ||
			got[0].AuthorOrg.ID != b1.ViewerOrg() || got[0].AuthorOrg.Name != "Deals Chat B1" {
			t.Fatalf("got %+v", got)
		}
		if n := countReads(t, offerL1, l1.UserID); n != 1 {
			t.Fatalf("message_reads L1 = %d, want 1", n)
		}
		offer, err := offers.Get(ctx, l1, offerL1)
		if err != nil {
			t.Fatal(err)
		}
		assertUnread(t, "L1 после чтения", offer.UnreadMessages, 0)
	})

	t.Run("уведомление message_received другой стороне", func(t *testing.T) {
		send(t, l1, offerL1, "Добрый день")
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM notifications n JOIN users u ON u.id = n.user_id
			WHERE n.type = 'message_received' AND n.entity_type = 'offer' AND n.entity_id = $1 AND u.organization_id = $2`,
			offerL1.String(), b1.ViewerOrg()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("уведомлений заёмщику = %d, want 1", n)
		}
	})

	t.Run("polling: after_id второго из трёх — ровно третье", func(t *testing.T) {
		appID := insertDealsApp(ctx, t, tx, b1)
		offerID := insertDealsOffer(ctx, t, tx, appID, l1, nil)
		send(t, b1, offerID, "1")
		second := send(t, l1, offerID, "2")
		third := send(t, b1, offerID, "3")
		got, err := chat.List(ctx, l1, offerID, second.ID, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != third.ID || got[0].Text != "3" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("чужие — NOT_FOUND на чтение и запись", func(t *testing.T) {
		for name, p := range map[string]service.Principal{"кредитор L2": l2, "заёмщик B2": b2} {
			_, err := chat.List(ctx, p, offerL1, 0, 50)
			assertCode(t, name+" list", err, apperr.CodeNotFound)
			_, err = chat.Send(ctx, p, offerL1, "чужое")
			assertCode(t, name+" send", err, apperr.CodeNotFound)
		}
	})

	t.Run("admin читает без указателя, писать не может", func(t *testing.T) {
		got, err := chat.List(ctx, admin, offerL1, 0, 50)
		if err != nil || len(got) == 0 {
			t.Fatalf("got %d, err = %v", len(got), err)
		}
		if n := countReads(t, offerL1, admin.UserID); n != 0 {
			t.Fatalf("message_reads admin = %d, want 0", n)
		}
		_, err = chat.Send(ctx, admin, offerL1, "модерация")
		assertCode(t, "admin send", err, apperr.CodeForbidden)
	})

	t.Run("withdrawn — INVALID_STATUS_TRANSITION, accepted — ок", func(t *testing.T) {
		appID := insertDealsApp(ctx, t, tx, b1)
		withdrawn := insertDealsOffer(ctx, t, tx, appID, l1, nil)
		if _, err := offers.Withdraw(ctx, l1, service.ClientMeta{}, withdrawn); err != nil {
			t.Fatal(err)
		}
		_, err := chat.Send(ctx, b1, withdrawn, "ещё тут?")
		assertCode(t, "withdrawn", err, apperr.CodeInvalidTransition)

		accepted := insertDealsOffer(ctx, t, tx, appID, l2, nil)
		deals := service.NewDeals(service.DealsDeps{Store: q, Tx: txm})
		if _, err := deals.AcceptOffer(ctx, b1, service.ClientMeta{}, accepted); err != nil {
			t.Fatal(err)
		}
		send(t, b1, accepted, "Реквизиты для перевода")
	})
}

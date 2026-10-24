package notifications_test

// Интеграционный тест ленты уведомлений на настоящем Postgres: фильтр unread_only, total и
// unread_total, порядок created_at DESC, id DESC, отметка чужого уведомления → 404.
// Всё в одной транзакции с откатом в Cleanup: created_at у всех строк одинаковый, поэтому
// порядок держит только тай-брейк по id.

import (
	"context"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/notifications"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestNotificationsIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	q := repository.New(tx)
	svc := notifications.NewNotifications(q, service.NewTxManager(tx))

	owner := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleBorrower, "Notif Owner")
	stranger := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleLender, "Notif Stranger")
	create := func(t *testing.T, p service.Principal, title string) int64 {
		t.Helper()
		n, err := q.CreateNotification(ctx, repository.CreateNotificationParams{
			UserID: p.UserID, Type: repository.NotificationTypeOfferReceived, Title: title,
			EntityType: repository.EntityTypeOffer, EntityID: "offer-" + title,
		})
		if err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	var ownIDs []int64
	for _, title := range []string{"1", "2", "3", "4"} {
		ownIDs = append(ownIDs, create(t, owner, title))
	}
	strangerID := create(t, stranger, "чужое")
	if err := svc.MarkRead(ctx, owner, ownIDs[0]); err != nil {
		t.Fatal(err)
	}

	t.Run("unread_only: страница из 2, total 3, unread_total 3", func(t *testing.T) {
		page, err := svc.List(ctx, owner, notifications.NotificationFilter{UnreadOnly: true, Page: 1, PageSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 2 || page.Total != 3 || page.UnreadTotal != 3 {
			t.Fatalf("items %d, total %d, unread %d", len(page.Items), page.Total, page.UnreadTotal)
		}
	})

	t.Run("вся лента: total 4, unread_total 3, новые сверху по id", func(t *testing.T) {
		page, err := svc.List(ctx, owner, notifications.NotificationFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 4 || page.UnreadTotal != 3 || len(page.Items) != 4 {
			t.Fatalf("items %d, total %d, unread %d", len(page.Items), page.Total, page.UnreadTotal)
		}
		for i, n := range page.Items {
			if want := ownIDs[len(ownIDs)-1-i]; n.ID != want {
				t.Fatalf("items[%d].id = %d, want %d", i, n.ID, want)
			}
		}
	})

	t.Run("чужое уведомление — 404, у владельца остаётся непрочитанным", func(t *testing.T) {
		err := svc.MarkRead(ctx, owner, strangerID)
		if e, ok := apperr.As(err); !ok || e.Code != apperr.CodeNotFound {
			t.Fatalf("err = %v, want NOT_FOUND", err)
		}
		page, err := svc.List(ctx, stranger, notifications.NotificationFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if page.UnreadTotal != 1 || page.Items[0].IsRead {
			t.Fatalf("stranger unread %d, items %+v", page.UnreadTotal, page.Items)
		}
	})

	t.Run("своё — unread_total меньше на 1, повтор идемпотентен", func(t *testing.T) {
		for range 2 {
			if err := svc.MarkRead(ctx, owner, ownIDs[1]); err != nil {
				t.Fatal(err)
			}
		}
		page, err := svc.List(ctx, owner, notifications.NotificationFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if page.UnreadTotal != 2 {
			t.Fatalf("unread_total = %d, want 2", page.UnreadTotal)
		}
	})

	t.Run("read-all — только свои", func(t *testing.T) {
		if err := svc.MarkAllRead(ctx, owner); err != nil {
			t.Fatal(err)
		}
		own, err := svc.List(ctx, owner, notifications.NotificationFilter{})
		if err != nil {
			t.Fatal(err)
		}
		other, err := svc.List(ctx, stranger, notifications.NotificationFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if own.UnreadTotal != 0 || other.UnreadTotal != 1 {
			t.Fatalf("owner unread %d, stranger unread %d", own.UnreadTotal, other.UnreadTotal)
		}
	})
}

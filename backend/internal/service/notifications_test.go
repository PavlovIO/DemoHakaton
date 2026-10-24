package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeNotificationStore отдаёт заданные ответы и запоминает аргументы вызовов.
type fakeNotificationStore struct {
	items       []repository.Notification
	total       int64
	unreadTotal int64
	markedRows  int64

	gotList   repository.ListNotificationsParams
	gotCount  repository.CountNotificationsParams
	gotUnread uuid.UUID
	gotMark   repository.MarkNotificationReadParams
	gotAll    uuid.UUID
}

func (s *fakeNotificationStore) ListNotifications(_ context.Context, arg repository.ListNotificationsParams) ([]repository.Notification, error) {
	s.gotList = arg
	return s.items, nil
}

func (s *fakeNotificationStore) CountNotifications(_ context.Context, arg repository.CountNotificationsParams) (int64, error) {
	s.gotCount = arg
	return s.total, nil
}

func (s *fakeNotificationStore) CountUnreadNotifications(_ context.Context, userID uuid.UUID) (int64, error) {
	s.gotUnread = userID
	return s.unreadTotal, nil
}

func (s *fakeNotificationStore) MarkNotificationRead(_ context.Context, arg repository.MarkNotificationReadParams) (int64, error) {
	s.gotMark = arg
	return s.markedRows, nil
}

func (s *fakeNotificationStore) MarkAllNotificationsRead(_ context.Context, userID uuid.UUID) error {
	s.gotAll = userID
	return nil
}

func TestNotificationsList(t *testing.T) {
	store := &fakeNotificationStore{
		items: []repository.Notification{{ID: 7, Type: repository.NotificationTypeOfferReceived}},
		total: 3, unreadTotal: 5,
	}
	p := service.Principal{UserID: uuid.New(), Role: "borrower"}

	page, err := service.NewNotifications(store).List(context.Background(), p,
		service.NotificationFilter{UnreadOnly: true, Page: 2, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}

	if page.Total != 3 || page.UnreadTotal != 5 || page.Page != 2 || page.PageSize != 2 ||
		len(page.Items) != 1 || page.Items[0].ID != 7 {
		t.Fatalf("page = %+v", page)
	}
	wantList := repository.ListNotificationsParams{
		UserID: p.UserID, Limit: 2, Offset: 2, UnreadOnly: pgtype.Bool{Bool: true, Valid: true},
	}
	if store.gotList != wantList {
		t.Fatalf("ListNotifications(%+v), want %+v", store.gotList, wantList)
	}
	wantCount := repository.CountNotificationsParams{UserID: p.UserID, UnreadOnly: wantList.UnreadOnly}
	if store.gotCount != wantCount || store.gotUnread != p.UserID {
		t.Fatalf("CountNotifications(%+v), CountUnreadNotifications(%s)", store.gotCount, store.gotUnread)
	}
}

func TestNotificationsListDefaults(t *testing.T) {
	store := &fakeNotificationStore{}
	p := service.Principal{UserID: uuid.New(), Role: "admin"}

	page, err := service.NewNotifications(store).List(context.Background(), p, service.NotificationFilter{})
	if err != nil {
		t.Fatal(err)
	}

	// unread_only не задан — фильтра нет: в SQL уходит NULL, а не false.
	if page.Page != 1 || page.PageSize != 20 || store.gotList.Limit != 20 || store.gotList.Offset != 0 ||
		store.gotList.UnreadOnly.Valid {
		t.Fatalf("page = %+v, list params = %+v", page, store.gotList)
	}
}

func TestNotificationsMarkRead(t *testing.T) {
	tests := []struct {
		name     string
		rows     int64
		wantCode apperr.Code
	}{
		{name: "своё уведомление", rows: 1},
		{name: "чужое или несуществующее", rows: 0, wantCode: apperr.CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeNotificationStore{markedRows: tt.rows}
			p := service.Principal{UserID: uuid.New(), Role: "lender"}

			err := service.NewNotifications(store).MarkRead(context.Background(), p, 42)

			if tt.wantCode == "" && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tt.wantCode != "" {
				if e, ok := apperr.As(err); !ok || e.Code != tt.wantCode {
					t.Fatalf("err = %v, want %s", err, tt.wantCode)
				}
			}
			if store.gotMark != (repository.MarkNotificationReadParams{ID: 42, UserID: p.UserID}) {
				t.Fatalf("MarkNotificationRead(%+v), want id 42 и user_id принципала", store.gotMark)
			}
		})
	}
}

func TestNotificationsMarkAllRead(t *testing.T) {
	store := &fakeNotificationStore{}
	p := service.Principal{UserID: uuid.New(), Role: "borrower"}

	if err := service.NewNotifications(store).MarkAllRead(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	if store.gotAll != p.UserID {
		t.Fatalf("MarkAllNotificationsRead(%s), want %s", store.gotAll, p.UserID)
	}
}

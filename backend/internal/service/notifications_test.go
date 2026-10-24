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
	repository.Querier
	items      []repository.Notification
	counts     repository.CountNotificationsRow
	markedRows int64

	gotList  repository.ListNotificationsParams
	gotCount uuid.UUID
	gotMark  repository.MarkNotificationReadParams
	gotAll   uuid.UUID
}

func (s *fakeNotificationStore) ListNotifications(_ context.Context, arg repository.ListNotificationsParams) ([]repository.Notification, error) {
	s.gotList = arg
	return s.items, nil
}

func (s *fakeNotificationStore) CountNotifications(_ context.Context, userID uuid.UUID) (repository.CountNotificationsRow, error) {
	s.gotCount = userID
	return s.counts, nil
}

func (s *fakeNotificationStore) MarkNotificationRead(_ context.Context, arg repository.MarkNotificationReadParams) (int64, error) {
	s.gotMark = arg
	return s.markedRows, nil
}

func (s *fakeNotificationStore) MarkAllNotificationsRead(_ context.Context, userID uuid.UUID) error {
	s.gotAll = userID
	return nil
}

// fakeNotificationSnapshot — снимок поверх той же памяти; считает вызовы.
type fakeNotificationSnapshot struct {
	store *fakeNotificationStore
	calls int
}

func (f *fakeNotificationSnapshot) ReadSnapshot(_ context.Context, fn func(q repository.Querier) error) error {
	f.calls++
	return fn(f.store)
}

func newNotifications(store *fakeNotificationStore) (*service.Notifications, *fakeNotificationSnapshot) {
	snapshot := &fakeNotificationSnapshot{store: store}
	return service.NewNotifications(store, snapshot), snapshot
}

func TestNotificationsList(t *testing.T) {
	tests := []struct {
		name       string
		unreadOnly bool
		wantTotal  int64
		wantFilter pgtype.Bool
	}{
		// total — с тем же фильтром, что и items; unread_total от фильтра не зависит.
		{name: "вся лента", wantTotal: 8},
		{name: "unread_only", unreadOnly: true, wantTotal: 5, wantFilter: pgtype.Bool{Bool: true, Valid: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeNotificationStore{
				items:  []repository.Notification{{ID: 7, Type: repository.NotificationTypeOfferReceived}},
				counts: repository.CountNotificationsRow{Total: 8, Unread: 5},
			}
			svc, snapshot := newNotifications(store)
			p := service.Principal{UserID: uuid.New(), Role: "borrower"}

			page, err := svc.List(context.Background(), p,
				service.NotificationFilter{UnreadOnly: tt.unreadOnly, Page: 2, PageSize: 2})
			if err != nil {
				t.Fatal(err)
			}

			if page.Total != tt.wantTotal || page.UnreadTotal != 5 || page.Page != 2 || page.PageSize != 2 ||
				len(page.Items) != 1 || page.Items[0].ID != 7 {
				t.Fatalf("page = %+v", page)
			}
			wantList := repository.ListNotificationsParams{UserID: p.UserID, Limit: 2, Offset: 2, UnreadOnly: tt.wantFilter}
			if store.gotList != wantList {
				t.Fatalf("ListNotifications(%+v), want %+v", store.gotList, wantList)
			}
			if store.gotCount != p.UserID {
				t.Fatalf("CountNotifications(%s), want %s", store.gotCount, p.UserID)
			}
			if snapshot.calls != 1 {
				t.Fatalf("ReadSnapshot calls = %d, want 1: элементы и счётчики — в одном снимке", snapshot.calls)
			}
		})
	}
}

func TestNotificationsListDefaults(t *testing.T) {
	store := &fakeNotificationStore{}
	svc, _ := newNotifications(store)
	p := service.Principal{UserID: uuid.New(), Role: "admin"}

	page, err := svc.List(context.Background(), p, service.NotificationFilter{})
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

			svc, _ := newNotifications(store)
			err := svc.MarkRead(context.Background(), p, 42)

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

	svc, _ := newNotifications(store)
	if err := svc.MarkAllRead(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	if store.gotAll != p.UserID {
		t.Fatalf("MarkAllNotificationsRead(%s), want %s", store.gotAll, p.UserID)
	}
}

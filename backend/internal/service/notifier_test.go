package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeNotificationWriter запоминает параметры запросов и отдаёт заданную ошибку.
type fakeNotificationWriter struct {
	err       error
	gotAdmins *repository.CreateNotificationsForAdminsParams
	gotOrg    *repository.CreateNotificationsForOrganizationParams
}

func (w *fakeNotificationWriter) CreateNotificationsForAdmins(_ context.Context, arg repository.CreateNotificationsForAdminsParams) (int64, error) {
	w.gotAdmins = &arg
	return 1, w.err
}

func (w *fakeNotificationWriter) CreateNotificationsForOrganization(_ context.Context, arg repository.CreateNotificationsForOrganizationParams) (int64, error) {
	w.gotOrg = &arg
	return 1, w.err
}

var testNotification = service.Notification{
	Type:       repository.NotificationTypeApplicationSubmitted,
	Title:      "Заявка APP-000123 на модерации",
	EntityType: repository.EntityTypeApplication,
	EntityID:   "0b6f7c4e-0000-4000-8000-000000000001",
}

func TestNotifierNotifyAdmins(t *testing.T) {
	w := &fakeNotificationWriter{}

	if err := (service.Notifier{}).NotifyAdmins(context.Background(), w, testNotification); err != nil {
		t.Fatal(err)
	}

	want := repository.CreateNotificationsForAdminsParams{
		Type: testNotification.Type, Title: testNotification.Title,
		EntityType: testNotification.EntityType, EntityID: testNotification.EntityID,
	}
	if w.gotAdmins == nil || *w.gotAdmins != want || w.gotOrg != nil {
		t.Fatalf("admins %+v, org %+v", w.gotAdmins, w.gotOrg)
	}
}

func TestNotifierNotifyOrganization(t *testing.T) {
	w := &fakeNotificationWriter{}
	orgID := uuid.New()

	if err := (service.Notifier{}).NotifyOrganization(context.Background(), w, orgID, testNotification); err != nil {
		t.Fatal(err)
	}

	want := repository.CreateNotificationsForOrganizationParams{
		Type: testNotification.Type, Title: testNotification.Title,
		EntityType: testNotification.EntityType, EntityID: testNotification.EntityID, OrganizationID: orgID,
	}
	if w.gotOrg == nil || *w.gotOrg != want || w.gotAdmins != nil {
		t.Fatalf("org %+v, admins %+v", w.gotOrg, w.gotAdmins)
	}
}

func TestNotifierWrapsError(t *testing.T) {
	dbErr := errors.New("insert failed")
	tests := []struct {
		name   string
		notify func(service.NotificationWriter) error
	}{
		{"NotifyAdmins", func(w service.NotificationWriter) error {
			return service.Notifier{}.NotifyAdmins(context.Background(), w, testNotification)
		}},
		{"NotifyOrganization", func(w service.NotificationWriter) error {
			return service.Notifier{}.NotifyOrganization(context.Background(), w, uuid.New(), testNotification)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.notify(&fakeNotificationWriter{err: dbErr})

			if !errors.Is(err, dbErr) || err.Error() == dbErr.Error() {
				t.Fatalf("err = %v, want обёрнутую %v", err, dbErr)
			}
		})
	}
}

package admin_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestAdminApproveApplication(t *testing.T) {
	t.Run("moderation — published, история, уведомление заёмщику, аудит", func(t *testing.T) {
		db := newFakeDB()
		app := db.withApplication(repository.ApplicationStatusModeration)
		db.openHighFlags = 1 // публикацию флаги не блокируют (DECISIONS п. 13)
		p := adminPrincipal()

		got, err := newAdmin(db).ApproveApplication(context.Background(), p, app.ID, servicetest.Ptr("ок"), servicetest.MetaFromIP("10.0.0.1"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Status != lending.AppStatusPublished || got.PublishedAt == nil || got.Borrower.Name != "ООО Альфа" {
			t.Fatalf("application = %+v", got)
		}
		if db.approvedApp.FromStatus != repository.ApplicationStatusModeration {
			t.Fatalf("from_status = %s", db.approvedApp.FromStatus)
		}
		h := db.history
		if len(h) != 1 || h[0].EntityType != repository.EntityTypeApplication || h[0].FromStatus.String != "moderation" ||
			h[0].ToStatus != "published" || h[0].ActorRole.UserRole != repository.UserRoleAdmin || h[0].Comment.String != "ок" {
			t.Fatalf("status_history = %+v", h)
		}
		if len(db.orgNotices) != 1 || db.orgNotices[0].Type != repository.NotificationTypeApplicationPublished ||
			db.orgNotices[0].OrganizationID != app.BorrowerOrganizationID {
			t.Fatalf("notifications = %+v", db.orgNotices)
		}
		if len(db.audit) != 1 || db.audit[0].Action != "application.approve" {
			t.Fatalf("audit = %+v", db.audit)
		}
	})

	for _, st := range []repository.ApplicationStatus{repository.ApplicationStatusDraft, repository.ApplicationStatusPublished} {
		t.Run(string(st)+" — 409 INVALID_STATUS_TRANSITION", func(t *testing.T) {
			db := newFakeDB()
			app := db.withApplication(st)
			_, err := newAdmin(db).ApproveApplication(context.Background(), adminPrincipal(), app.ID, nil, service.ClientMeta{})
			servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
			if db.approvedApp != nil {
				t.Fatal("ApproveApplication вызван")
			}
		})
	}

	t.Run("статус сменился под охраной UPDATE — 409", func(t *testing.T) {
		db := newFakeDB()
		app := db.withApplication(repository.ApplicationStatusModeration)
		db.raceStatus = true
		_, err := newAdmin(db).ApproveApplication(context.Background(), adminPrincipal(), app.ID, nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
	})

	t.Run("нет заявки — 404", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).ApproveApplication(context.Background(), adminPrincipal(), uuid.New(), nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("не админ — 403", func(t *testing.T) {
		db := newFakeDB()
		app := db.withApplication(repository.ApplicationStatusModeration)
		_, err := newAdmin(db).ApproveApplication(context.Background(), borrowerPrincipal(), app.ID, nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

func TestAdminRejectApplication(t *testing.T) {
	t.Run("moderation — rejected, причина в moderation_comment и истории", func(t *testing.T) {
		db := newFakeDB()
		app := db.withApplication(repository.ApplicationStatusModeration)

		got, err := newAdmin(db).RejectApplication(context.Background(), adminPrincipal(), app.ID, "уточните цель", service.ClientMeta{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Status != lending.AppStatusRejected || got.ModerationComment == nil || *got.ModerationComment != "уточните цель" {
			t.Fatalf("application = %+v", got)
		}
		if len(db.history) != 1 || db.history[0].ToStatus != "rejected" || db.history[0].Comment.String != "уточните цель" {
			t.Fatalf("status_history = %+v", db.history)
		}
		if len(db.orgNotices) != 1 || db.orgNotices[0].Type != repository.NotificationTypeApplicationRejected {
			t.Fatalf("notifications = %+v", db.orgNotices)
		}
		if len(db.audit) != 1 || db.audit[0].Action != "application.reject" {
			t.Fatalf("audit = %+v", db.audit)
		}
	})

	t.Run("published — 409", func(t *testing.T) {
		db := newFakeDB()
		app := db.withApplication(repository.ApplicationStatusPublished)
		_, err := newAdmin(db).RejectApplication(context.Background(), adminPrincipal(), app.ID, "уточните цель", service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
	})
}

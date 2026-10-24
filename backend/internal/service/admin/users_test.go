package admin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestBlockUser(t *testing.T) {
	t.Run("borrower — blocked, сессии удалены, история, аудит, без уведомления", func(t *testing.T) {
		db := newFakeDB()
		u := db.withUser(repository.UserRoleBorrower, repository.UserStatusActive)
		p := adminPrincipal()

		got, err := newAdmin(db).BlockUser(context.Background(), p, u.ID, "мошенничество", service.ClientMeta{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Status != "blocked" || got.OrganizationID == nil || got.Email != u.Email {
			t.Fatalf("user = %+v", got)
		}
		if db.blocked == nil || db.blocked.BlockedReason.String != "мошенничество" {
			t.Fatalf("BlockUser = %+v", db.blocked)
		}
		if len(db.deletedSessions) != 1 || db.deletedSessions[0] != u.ID {
			t.Fatalf("deleted sessions = %v", db.deletedSessions)
		}
		h := db.history
		if len(h) != 1 || h[0].EntityType != repository.EntityTypeUser || h[0].EntityID != u.ID ||
			h[0].FromStatus.String != "active" || h[0].ToStatus != "blocked" || h[0].Comment.String != "мошенничество" ||
			h[0].ActorUserID.Bytes != p.UserID {
			t.Fatalf("status_history = %+v", h)
		}
		if len(db.audit) != 1 || db.audit[0].Action != "user.block" || !strings.Contains(string(db.audit[0].Meta), "мошенничество") {
			t.Fatalf("audit = %+v", db.audit)
		}
		if len(db.orgNotices) != 0 {
			t.Fatalf("notifications = %+v, для блокировки уведомления нет", db.orgNotices)
		}
	})

	t.Run("уже заблокирован — 200 без новых записей", func(t *testing.T) {
		db := newFakeDB()
		u := db.withUser(repository.UserRoleLender, repository.UserStatusBlocked)
		got, err := newAdmin(db).BlockUser(context.Background(), adminPrincipal(), u.ID, "повторно", service.ClientMeta{})
		if err != nil || got.Status != "blocked" {
			t.Fatalf("user = %+v, err = %v", got, err)
		}
		if db.blocked != nil || len(db.history)+len(db.audit)+len(db.deletedSessions) > 0 {
			t.Fatalf("записи при повторной блокировке: block=%v history=%d audit=%d sessions=%d",
				db.blocked, len(db.history), len(db.audit), len(db.deletedSessions))
		}
	})

	t.Run("администратор — 403, в том числе сам себя", func(t *testing.T) {
		db := newFakeDB()
		other := db.withUser(repository.UserRoleAdmin, repository.UserStatusActive)
		p := adminPrincipal()
		db.users[p.UserID] = repository.User{ID: p.UserID, Role: repository.UserRoleAdmin, Status: repository.UserStatusActive}
		for _, id := range []uuid.UUID{other.ID, p.UserID} {
			_, err := newAdmin(db).BlockUser(context.Background(), p, id, "по ошибке", service.ClientMeta{})
			servicetest.WantCode(t, err, apperr.CodeForbidden)
		}
		if db.blocked != nil {
			t.Fatal("BlockUser вызван для администратора")
		}
	})

	t.Run("нет пользователя — 404", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).BlockUser(context.Background(), adminPrincipal(), uuid.New(), "нет", service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("не админ — 403", func(t *testing.T) {
		db := newFakeDB()
		u := db.withUser(repository.UserRoleLender, repository.UserStatusActive)
		_, err := newAdmin(db).BlockUser(context.Background(), borrowerPrincipal(), u.ID, "конкурент", service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

func TestUnblockUser(t *testing.T) {
	t.Run("blocked — active, история, аудит, сессии не трогаются", func(t *testing.T) {
		db := newFakeDB()
		u := db.withUser(repository.UserRoleBorrower, repository.UserStatusBlocked)
		got, err := newAdmin(db).UnblockUser(context.Background(), adminPrincipal(), u.ID, service.ClientMeta{})
		if err != nil || got.Status != "active" {
			t.Fatalf("user = %+v, err = %v", got, err)
		}
		if len(db.history) != 1 || db.history[0].FromStatus.String != "blocked" || db.history[0].ToStatus != "active" {
			t.Fatalf("status_history = %+v", db.history)
		}
		if len(db.audit) != 1 || db.audit[0].Action != "user.unblock" || len(db.deletedSessions) != 0 {
			t.Fatalf("audit = %+v, sessions = %v", db.audit, db.deletedSessions)
		}
	})

	t.Run("уже активен — 200 без новых записей", func(t *testing.T) {
		db := newFakeDB()
		u := db.withUser(repository.UserRoleBorrower, repository.UserStatusActive)
		got, err := newAdmin(db).UnblockUser(context.Background(), adminPrincipal(), u.ID, service.ClientMeta{})
		if err != nil || got.Status != "active" || db.unblocked || len(db.history) > 0 {
			t.Fatalf("user = %+v, err = %v, unblocked = %v", got, err, db.unblocked)
		}
	})

	t.Run("админ — 403 без записей", func(t *testing.T) {
		db := newFakeDB()
		u := db.withUser(repository.UserRoleAdmin, repository.UserStatusActive)
		_, err := newAdmin(db).UnblockUser(context.Background(), adminPrincipal(), u.ID, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
		if db.unblocked || len(db.history) > 0 || len(db.audit) > 0 {
			t.Fatalf("unblocked = %v, history = %v, audit = %v", db.unblocked, db.history, db.audit)
		}
	})

	t.Run("нет пользователя — 404", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).UnblockUser(context.Background(), adminPrincipal(), uuid.New(), service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})
}

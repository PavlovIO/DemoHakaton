package admin_test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// Журнал на заселённой БД: уникальный action отделяет записи теста от демо-сида.
func TestListAuditLogIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	adm := insertAdmin(ctx, t, tx)
	author := servicetest.InsertUser(ctx, t, tx, repository.UserRoleAdmin, nil)
	other := servicetest.InsertUser(ctx, t, tx, repository.UserRoleAdmin, nil)
	action := "test." + uuid.NewString()
	ip := netip.MustParseAddr("198.51.100.9")
	meta := service.ClientMeta{IP: &ip, UserAgent: "audit-it"}

	write := func(userID uuid.UUID, entityID string, at time.Time) {
		t.Helper()
		err := service.WriteAudit(ctx, repository.New(tx), meta, service.AuditEvent{
			Action: action, UserID: &userID, EntityType: repository.EntityTypeOffer, EntityID: entityID,
			Meta: map[string]any{"reason": "it"},
		})
		if err != nil {
			t.Fatalf("WriteAudit: %v", err)
		}
		// created_at = now() транзакции, одинаковый у всех трёх: сдвигаем вручную для фильтра периода.
		if _, err := tx.Exec(ctx, `UPDATE audit_log SET created_at = $1
			WHERE id = (SELECT max(id) FROM audit_log WHERE action = $2)`, at, action); err != nil {
			t.Fatalf("shift created_at: %v", err)
		}
	}
	old := time.Date(2001, 3, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2001, 6, 1, 0, 0, 0, 0, time.UTC)
	write(author, "offer-1", old)
	write(author, "offer-2", recent)
	write(other, "offer-3", recent)

	svc := newIntegrationAdmin(tx)

	t.Run("by action", func(t *testing.T) {
		page, err := svc.ListAuditLog(ctx, adm, admin.AuditLogFilter{Action: &action, PageSize: 2})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if page.Total != 3 || len(page.Items) != 2 {
			t.Fatalf("total=%d items=%d, want 3 and 2", page.Total, len(page.Items))
		}
		// Новые сверху: первой идёт одна из записей июня.
		first := page.Items[0]
		if !first.CreatedAt.Equal(recent) {
			t.Errorf("first created_at = %v, want %v", first.CreatedAt, recent)
		}
		if first.IP == nil || *first.IP != ip || first.UserAgent == nil || *first.UserAgent != "audit-it" {
			t.Errorf("client = %v/%v, want %v/audit-it", first.IP, first.UserAgent, ip)
		}
		if first.EntityType == nil || *first.EntityType != "offer" || first.Meta["reason"] != "it" {
			t.Errorf("entity/meta = %v/%v", first.EntityType, first.Meta)
		}
	})

	t.Run("by user and created_from", func(t *testing.T) {
		from := time.Date(2001, 5, 1, 0, 0, 0, 0, time.UTC)
		page, err := svc.ListAuditLog(ctx, adm, admin.AuditLogFilter{Action: &action, UserID: &author, CreatedFrom: &from})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if page.Total != 1 || len(page.Items) != 1 || *page.Items[0].EntityID != "offer-2" {
			t.Fatalf("page = %+v, want only offer-2", page)
		}
	})

	t.Run("created_to is exclusive", func(t *testing.T) {
		entityType := "offer"
		page, err := svc.ListAuditLog(ctx, adm, admin.AuditLogFilter{Action: &action, EntityType: &entityType, CreatedTo: &recent})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if page.Total != 1 || *page.Items[0].EntityID != "offer-1" {
			t.Fatalf("page = %+v, want only offer-1", page)
		}
	})

	t.Run("by entity_id", func(t *testing.T) {
		entityID := "offer-3"
		page, err := svc.ListAuditLog(ctx, adm, admin.AuditLogFilter{Action: &action, EntityID: &entityID})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if page.Total != 1 || *page.Items[0].UserID != other {
			t.Fatalf("page = %+v, want offer-3 by other", page)
		}
	})
}

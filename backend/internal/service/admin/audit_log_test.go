package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestListAuditLog(t *testing.T) {
	t.Run("не админ — 403", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).ListAuditLog(context.Background(), borrowerPrincipal(), admin.AuditLogFilter{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})

	t.Run("from > to — 400 created_to", func(t *testing.T) {
		from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		_, err := newAdmin(newFakeDB()).ListAuditLog(context.Background(), adminPrincipal(),
			admin.AuditLogFilter{CreatedFrom: &from, CreatedTo: &to})
		servicetest.WantValidationField(t, err, "created_to")
	})
}

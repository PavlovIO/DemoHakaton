package admin_test

import (
	"context"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestListAuditLog(t *testing.T) {
	t.Run("не админ — 403", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).ListAuditLog(context.Background(), borrowerPrincipal(), admin.AuditLogFilter{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

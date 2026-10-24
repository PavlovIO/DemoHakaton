package admin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func TestResolveAmlFlag(t *testing.T) {
	t.Run("open → false_positive: решение, комментарий, аудит", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		flag := db.withAmlFlag(org, repository.AmlFlagStatusOpen)
		p := adminPrincipal()

		got, err := newAdmin(db).ResolveAmlFlag(context.Background(), p, flag.ID,
			repository.AmlFlagStatusFalsePositive, "однофамилец", servicetest.MetaFromIP("10.0.0.1"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Status != "false_positive" || got.ResolvedAt == nil || got.ResolvedByUserID == nil ||
			*got.ResolvedByUserID != p.UserID || got.ResolutionComment == nil || *got.ResolutionComment != "однофамилец" {
			t.Fatalf("flag = %+v", got)
		}
		if got.Organization.Name != org.Name || got.Organization.CountryCode != org.CountryCode {
			t.Fatalf("organization = %+v", got.Organization)
		}
		a := db.audit
		if len(a) != 1 || a[0].Action != "aml_flag.resolve" || a[0].EntityType.EntityType != repository.EntityTypeAmlFlag ||
			!strings.Contains(string(a[0].Meta), "false_positive") {
			t.Fatalf("audit = %+v", a)
		}
	})

	t.Run("уже confirmed — 409 INVALID_STATUS_TRANSITION, без аудита", func(t *testing.T) {
		db := newFakeDB()
		flag := db.withAmlFlag(db.withOrg(repository.KycStatusVerified), repository.AmlFlagStatusConfirmed)
		_, err := newAdmin(db).ResolveAmlFlag(context.Background(), adminPrincipal(), flag.ID,
			repository.AmlFlagStatusFalsePositive, "передумал", servicetest.MetaFromIP("10.0.0.1"))
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
		if len(db.audit) != 0 {
			t.Fatalf("audit = %+v", db.audit)
		}
	})

	t.Run("нет флага — 404", func(t *testing.T) {
		db := newFakeDB()
		_, err := newAdmin(db).ResolveAmlFlag(context.Background(), adminPrincipal(), 999,
			repository.AmlFlagStatusConfirmed, "подтверждаю", servicetest.MetaFromIP("10.0.0.1"))
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("не админ — 403", func(t *testing.T) {
		db := newFakeDB()
		flag := db.withAmlFlag(db.withOrg(repository.KycStatusUnderReview), repository.AmlFlagStatusOpen)
		_, err := newAdmin(db).ResolveAmlFlag(context.Background(), borrowerPrincipal(), flag.ID,
			repository.AmlFlagStatusConfirmed, "подтверждаю", servicetest.MetaFromIP("10.0.0.1"))
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

func TestListAmlFlags(t *testing.T) {
	t.Run("фильтры и пагинация доходят до запросов, чтение в одном снимке", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		db.withAmlFlag(org, repository.AmlFlagStatusOpen)
		db.total = 41
		appID := uuid.New()
		f := admin.AmlFlagFilter{
			Status: servicetest.Ptr("open"), Severity: servicetest.Ptr("high"),
			OrganizationID: &org.ID, ApplicationID: &appID, Page: 3, PageSize: 20,
		}

		got, err := newAdmin(db).ListAmlFlags(context.Background(), adminPrincipal(), f)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Total != 41 || got.Page != 3 || got.PageSize != 20 || len(got.Items) != 1 {
			t.Fatalf("page = %+v", got)
		}
		if got.Items[0].Organization.Name != org.Name {
			t.Fatalf("organization = %+v", got.Items[0].Organization)
		}
		l := db.flagListArg
		if l == nil || l.Limit != 20 || l.Offset != 40 ||
			l.Status.AmlFlagStatus != repository.AmlFlagStatusOpen || !l.Status.Valid ||
			l.Severity.AmlSeverity != repository.AmlSeverityHigh || !l.Severity.Valid ||
			l.OrganizationID.Bytes != org.ID || l.ApplicationID.Bytes != appID {
			t.Fatalf("ListAmlFlags arg = %+v", l)
		}
		c := db.flagCountArg
		if c == nil || c.Status != l.Status || c.Severity != l.Severity || c.OrganizationID != l.OrganizationID || c.ApplicationID != l.ApplicationID {
			t.Fatalf("CountAmlFlags arg = %+v, list = %+v", c, l)
		}
		if db.snapshots != 1 {
			t.Fatalf("snapshots = %d, страница и total — в одном снимке", db.snapshots)
		}
	})

	t.Run("без фильтров — всё, срез не nil", func(t *testing.T) {
		db := newFakeDB()
		db.withOrg(repository.KycStatusUnderReview)
		got, err := newAdmin(db).ListAmlFlags(context.Background(), adminPrincipal(), admin.AmlFlagFilter{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Items == nil {
			t.Fatal("Items = nil, в ответе нужен []")
		}
		l := db.flagListArg
		if l.Status.Valid || l.Severity.Valid || l.OrganizationID.Valid || l.ApplicationID.Valid {
			t.Fatalf("ListAmlFlags arg = %+v", l)
		}
	})

	t.Run("не админ — 403", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).ListAmlFlags(context.Background(), borrowerPrincipal(), admin.AmlFlagFilter{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

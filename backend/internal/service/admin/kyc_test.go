package admin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

func newAdmin(db *fakeDB) *admin.Admin {
	return admin.New(admin.Deps{Store: db, Tx: fakeTx{db: db}})
}

func TestApproveKyc(t *testing.T) {
	t.Run("under_review без флагов — verified, история, уведомление, аудит", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		p := adminPrincipal()

		got, err := newAdmin(db).ApproveKyc(context.Background(), p, org.ID, servicetest.Ptr("всё проверено"), servicetest.MetaFromIP("10.0.0.1"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Org.KycStatus != "verified" || got.Org.VerifiedAt == nil || got.Beneficiaries == nil {
			t.Fatalf("org = %+v, beneficiaries = %v", got.Org, got.Beneficiaries)
		}
		h := db.history
		if len(h) != 1 || h[0].EntityType != repository.EntityTypeOrganization || h[0].EntityID != org.ID ||
			h[0].FromStatus.String != "under_review" || h[0].ToStatus != "verified" ||
			h[0].ActorRole.UserRole != repository.UserRoleAdmin || h[0].ActorUserID.Bytes != p.UserID ||
			h[0].Comment.String != "всё проверено" {
			t.Fatalf("status_history = %+v", h)
		}
		if len(db.orgNotices) != 1 || db.orgNotices[0].Type != repository.NotificationTypeKycApproved ||
			db.orgNotices[0].OrganizationID != org.ID {
			t.Fatalf("notifications = %+v", db.orgNotices)
		}
		if len(db.audit) != 1 || db.audit[0].Action != "kyc.approve" || db.audit[0].Ip == nil ||
			db.audit[0].EntityID.String != org.ID.String() {
			t.Fatalf("audit = %+v", db.audit)
		}
	})

	t.Run("открытый high-флаг — 409 CONFLICT, ничего не записано", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		db.openHighFlags = 1

		_, err := newAdmin(db).ApproveKyc(context.Background(), adminPrincipal(), org.ID, nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeConflict)
		if db.approvedKyc || len(db.history)+len(db.orgNotices)+len(db.audit) > 0 {
			t.Fatalf("записи при 409: approved=%v history=%d notices=%d audit=%d",
				db.approvedKyc, len(db.history), len(db.orgNotices), len(db.audit))
		}
	})

	// Переход проверяется раньше флагов: verified с флагом — INVALID_STATUS_TRANSITION.
	for _, st := range []repository.KycStatus{repository.KycStatusDraft, repository.KycStatusVerified, repository.KycStatusRejected} {
		t.Run(string(st)+" — 409 INVALID_STATUS_TRANSITION", func(t *testing.T) {
			db := newFakeDB()
			org := db.withOrg(st)
			db.openHighFlags = 1
			_, err := newAdmin(db).ApproveKyc(context.Background(), adminPrincipal(), org.ID, nil, service.ClientMeta{})
			servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
		})
	}

	t.Run("статус сменился под охраной UPDATE — 409 INVALID_STATUS_TRANSITION", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		db.raceStatus = true
		_, err := newAdmin(db).ApproveKyc(context.Background(), adminPrincipal(), org.ID, nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
	})

	t.Run("нет компании — 404", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).ApproveKyc(context.Background(), adminPrincipal(), uuid.New(), nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("не админ — 403", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		_, err := newAdmin(db).ApproveKyc(context.Background(), borrowerPrincipal(), org.ID, nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
		if db.approvedKyc {
			t.Fatal("ApproveKyc вызван не админом")
		}
	})
}

func TestRejectKyc(t *testing.T) {
	t.Run("under_review — rejected с причиной, история, уведомление, аудит", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		db.openHighFlags = 1 // флаги отказу не мешают

		got, err := newAdmin(db).RejectKyc(context.Background(), adminPrincipal(), org.ID, "нет устава", service.ClientMeta{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Org.KycStatus != "rejected" || got.Org.KycRejectReason == nil || *got.Org.KycRejectReason != "нет устава" {
			t.Fatalf("org = %+v", got.Org)
		}
		if db.rejectedKyc == nil || db.rejectedKyc.KycRejectReason.String != "нет устава" {
			t.Fatalf("RejectKyc = %+v", db.rejectedKyc)
		}
		h := db.history
		if len(h) != 1 || h[0].FromStatus.String != "under_review" || h[0].ToStatus != "rejected" || h[0].Comment.String != "нет устава" {
			t.Fatalf("status_history = %+v", h)
		}
		if len(db.orgNotices) != 1 || db.orgNotices[0].Type != repository.NotificationTypeKycRejected {
			t.Fatalf("notifications = %+v", db.orgNotices)
		}
		if len(db.audit) != 1 || db.audit[0].Action != "kyc.reject" || !strings.Contains(string(db.audit[0].Meta), "нет устава") {
			t.Fatalf("audit = %+v", db.audit)
		}
	})

	t.Run("verified — 409 INVALID_STATUS_TRANSITION", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusVerified)
		_, err := newAdmin(db).RejectKyc(context.Background(), adminPrincipal(), org.ID, "нет устава", service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
		if db.rejectedKyc != nil {
			t.Fatal("RejectKyc вызван")
		}
	})

	t.Run("нет компании — 404", func(t *testing.T) {
		_, err := newAdmin(newFakeDB()).RejectKyc(context.Background(), adminPrincipal(), uuid.New(), "нет устава", service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("не админ — 403", func(t *testing.T) {
		db := newFakeDB()
		org := db.withOrg(repository.KycStatusUnderReview)
		_, err := newAdmin(db).RejectKyc(context.Background(), borrowerPrincipal(), org.ID, "нет устава", service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeForbidden)
	})
}

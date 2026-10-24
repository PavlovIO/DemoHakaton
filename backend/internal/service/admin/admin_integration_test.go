package admin_test

// Интеграционные тесты админки на настоящей БД:
//   - охрана статуса и FOR UPDATE — то, что фейк не моделирует (параллельные approve);
//   - 409 при high-флаге не меняет состояние БД;
//   - блокировка удаляет сессии, и старая cookie перестаёт работать;
//   - ESCAPE в поиске и фильтр kyc_statuses[].

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// adminDB — pgx.Tx или пул: Querier для сервиса и TxManager поверх того же соединения.
type adminDB interface {
	servicetest.RowQuerier
	repository.DBTX
	service.TxBeginner
}

func newIntegrationAdmin(db adminDB) *admin.Admin {
	return admin.New(admin.Deps{Tx: service.NewTxManager(db)})
}

// insertCompany — заёмщик из RU в статусе status с одним пользователем.
func insertCompany(ctx context.Context, t *testing.T, db servicetest.RowQuerier, name string, status repository.KycStatus) service.Principal {
	t.Helper()
	var orgID uuid.UUID
	err := db.QueryRow(ctx, `INSERT INTO organizations (type, name, country_code, registration_number, legal_address, kyc_status)
		VALUES ('borrower', $1, 'RU', $2, 'Москва', $3) RETURNING id`,
		name, servicetest.UniqueRegistrationNumber(), status).Scan(&orgID)
	if err != nil {
		t.Fatalf("organization %s: %v", name, err)
	}
	userID := servicetest.InsertUser(ctx, t, db, repository.UserRoleBorrower, &orgID)
	return service.Principal{UserID: userID, OrgID: &orgID, Role: "borrower", KycStatus: string(status)}
}

func insertAdmin(ctx context.Context, t *testing.T, db servicetest.RowQuerier) service.Principal {
	t.Helper()
	return service.Principal{UserID: servicetest.InsertUser(ctx, t, db, repository.UserRoleAdmin, nil), Role: "admin"}
}

// insertFlag — AML-флаг; закрытый (не open) получает resolved_* по aml_flags_resolution_chk.
func insertFlag(ctx context.Context, t *testing.T, db servicetest.RowQuerier, orgID uuid.UUID, severity, status string, resolver uuid.UUID) {
	t.Helper()
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO aml_flags (organization_id, rule, severity, details, status, resolved_by_user_id, resolved_at)
		VALUES ($1, 'sanctions_match', $2::aml_severity, 'совпадение', $3::aml_flag_status,
		        CASE WHEN $3::aml_flag_status = 'open' THEN NULL ELSE $4::uuid END,
		        CASE WHEN $3::aml_flag_status = 'open' THEN NULL ELSE now() END)
		RETURNING id`, orgID, severity, status, resolver).Scan(&id)
	if err != nil {
		t.Fatalf("aml flag: %v", err)
	}
}

func count(ctx context.Context, t *testing.T, db servicetest.RowQuerier, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func TestApproveKycIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("under_review без флагов — verified, история, уведомление, аудит с IP и UA", func(t *testing.T) {
		tx := servicetest.BeginTx(t)
		company := insertCompany(ctx, t, tx, "Approve OK", repository.KycStatusUnderReview)
		adm := insertAdmin(ctx, t, tx)

		got, err := newIntegrationAdmin(tx).ApproveKyc(ctx, adm, *company.OrgID, nil, servicetest.MetaFromIP("10.1.2.3"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Org.KycStatus != "verified" || got.Org.VerifiedAt == nil || got.Org.CountryCode != "RU" {
			t.Fatalf("org = %+v", got.Org)
		}
		if n := count(ctx, t, tx, `SELECT count(*) FROM status_history WHERE entity_type = 'organization' AND entity_id = $1
			AND from_status = 'under_review' AND to_status = 'verified' AND actor_role = 'admin' AND actor_user_id = $2`,
			*company.OrgID, adm.UserID); n != 1 {
			t.Fatalf("status_history = %d", n)
		}
		if n := count(ctx, t, tx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'kyc_approved'`, company.UserID); n != 1 {
			t.Fatalf("notifications = %d", n)
		}
		if n := count(ctx, t, tx, `SELECT count(*) FROM audit_log WHERE action = 'kyc.approve' AND entity_id = $1
			AND ip = '10.1.2.3' AND user_agent = 'ua'`, company.OrgID.String()); n != 1 {
			t.Fatalf("audit_log = %d", n)
		}
	})

	t.Run("открытый high-флаг — 409 CONFLICT, БД не изменилась", func(t *testing.T) {
		tx := servicetest.BeginTx(t)
		company := insertCompany(ctx, t, tx, "Approve Flag", repository.KycStatusUnderReview)
		adm := insertAdmin(ctx, t, tx)
		insertFlag(ctx, t, tx, *company.OrgID, "high", "open", adm.UserID)

		_, err := newIntegrationAdmin(tx).ApproveKyc(ctx, adm, *company.OrgID, nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeConflict)
		if n := count(ctx, t, tx, `SELECT count(*) FROM organizations WHERE id = $1 AND kyc_status = 'under_review'`, *company.OrgID); n != 1 {
			t.Fatal("статус изменился")
		}
		if n := count(ctx, t, tx, `SELECT (SELECT count(*) FROM status_history WHERE entity_id = $1)
			+ (SELECT count(*) FROM audit_log WHERE entity_id = $1::text)
			+ (SELECT count(*) FROM notifications WHERE user_id = $2)`, *company.OrgID, company.UserID); n != 0 {
			t.Fatalf("записей после 409: %d", n)
		}
	})

	t.Run("high-флаг закрыт как false_positive — 200", func(t *testing.T) {
		tx := servicetest.BeginTx(t)
		company := insertCompany(ctx, t, tx, "Approve FP", repository.KycStatusUnderReview)
		adm := insertAdmin(ctx, t, tx)
		insertFlag(ctx, t, tx, *company.OrgID, "high", "false_positive", adm.UserID)
		insertFlag(ctx, t, tx, *company.OrgID, "medium", "open", adm.UserID)

		if _, err := newIntegrationAdmin(tx).ApproveKyc(ctx, adm, *company.OrgID, nil, service.ClientMeta{}); err != nil {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("draft — 409 INVALID_STATUS_TRANSITION", func(t *testing.T) {
		tx := servicetest.BeginTx(t)
		company := insertCompany(ctx, t, tx, "Approve Draft", repository.KycStatusDraft)
		_, err := newIntegrationAdmin(tx).ApproveKyc(ctx, insertAdmin(ctx, t, tx), *company.OrgID, nil, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
	})
}

func TestRejectKycIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	company := insertCompany(ctx, t, tx, "Reject", repository.KycStatusUnderReview)

	if _, err := newIntegrationAdmin(tx).RejectKyc(ctx, insertAdmin(ctx, t, tx), *company.OrgID, "нет устава", service.ClientMeta{}); err != nil {
		t.Fatalf("err = %v", err)
	}

	// Причину видит сама компания: getMyOrganization.
	q := repository.New(tx)
	mine, err := kyc.NewKyc(kyc.KycDeps{Store: q, Tx: service.NewTxManager(tx), Screener: aml.NoopScreener{}}).MyOrganization(ctx, company)
	if err != nil {
		t.Fatalf("MyOrganization err = %v", err)
	}
	if mine.Org.KycStatus != "rejected" || mine.Org.KycRejectReason == nil || *mine.Org.KycRejectReason != "нет устава" {
		t.Fatalf("org = %+v", mine.Org)
	}
}

// TestApproveKycConcurrent — два параллельных approve одной компании: настоящие транзакции
// на пуле с коммитом. FOR UPDATE пропускает одно решение, второе видит verified и
// получает 409; в истории одна строка.
func TestApproveKycConcurrent(t *testing.T) {
	ctx := context.Background()
	pool := servicetest.NewPool(t)
	company := insertCompany(ctx, t, pool, "Concurrent approve", repository.KycStatusUnderReview)
	// Админ закоммичен в общую БД, пока идут тесты других пакетов: blocked, чтобы не попасть
	// в «каждому активному админу» (TestSubmitKycIntegration считает их). Сервису статус
	// principal не важен — его проверяет сессия.
	var adminID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, role, status)
		VALUES ($1, 'x', 'admin', 'blocked') RETURNING id`, "test-"+uuid.NewString()+"@example.test").Scan(&adminID); err != nil {
		t.Fatalf("admin: %v", err)
	}
	adm := service.Principal{UserID: adminID, Role: "admin"}
	t.Cleanup(func() { deleteCommitted(t, pool, *company.OrgID, adm.UserID) })

	svc := newIntegrationAdmin(pool)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.ApproveKyc(ctx, adm, *company.OrgID, nil, service.ClientMeta{})
		}()
	}
	wg.Wait()

	var ok, conflicts int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case apperr.CodeOf(err) == apperr.CodeInvalidTransition:
			conflicts++
		default:
			t.Fatalf("err = %v", err)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatalf("ok = %d, conflicts = %d; want 1 и 1", ok, conflicts)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM status_history WHERE entity_id = $1`, *company.OrgID); n != 1 {
		t.Fatalf("status_history = %d, want 1", n)
	}
}

// deleteCommitted — закоммиченные данные компании и админа. Тест ходит владельцем схемы
// (lp из DATABASE_URL), как TestAcceptOfferConcurrent.
func deleteCommitted(t *testing.T, pool *pgxpool.Pool, orgID, adminID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM status_history WHERE entity_id = $1 OR actor_user_id = $2`,
		`DELETE FROM audit_log WHERE user_id = $2 OR entity_id = $1::text`,
		`DELETE FROM users WHERE organization_id = $1 OR id = $2`, // notifications, sessions — CASCADE
		`DELETE FROM organizations WHERE id = $1 AND $2::uuid IS NOT NULL`,
	} {
		if _, err := pool.Exec(ctx, q, orgID, adminID); err != nil {
			t.Errorf("cleanup: %v; %s", err, q)
		}
	}
}

func TestModerationIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	borrower := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleBorrower, "Moderation B")
	lender := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleLender, "Moderation L")
	adm := insertAdmin(ctx, t, tx)
	insertApp := func() uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO applications (borrower_organization_id, created_by_user_id, status,
				amount, currency, target_country_code, purpose, term_months, transfer_method, submitted_at)
			VALUES ($1, $2, 'moderation', 500000.00, 'EUR', 'DE', 'Закупка оборудования', 12, 'sepa', now())
			RETURNING id`, borrower.ViewerOrg(), borrower.UserID).Scan(&id)
		if err != nil {
			t.Fatalf("application: %v", err)
		}
		return id
	}
	svc := newIntegrationAdmin(tx)

	t.Run("approve — заявка на витрине кредитора, уведомление заёмщику", func(t *testing.T) {
		appID := insertApp()
		got, err := svc.ApproveApplication(ctx, adm, appID, nil, service.ClientMeta{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Status != lending.AppStatusPublished || got.PublishedAt == nil {
			t.Fatalf("application = %+v", got)
		}
		apps := lending.NewApplications(lending.ApplicationsDeps{Store: repository.New(tx), Tx: service.NewTxManager(tx), Screener: aml.NoopScreener{}})
		page, err := apps.List(ctx, lender, lending.ApplicationFilter{Statuses: []lending.ApplicationStatus{lending.AppStatusPublished}, PageSize: 100})
		if err != nil {
			t.Fatalf("List err = %v", err)
		}
		found := false
		for _, a := range page.Items {
			found = found || a.ID == appID
		}
		if !found {
			t.Fatal("опубликованной заявки нет на витрине кредитора")
		}
		if n := count(ctx, t, tx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'application_published'`, borrower.UserID); n != 1 {
			t.Fatalf("notifications = %d", n)
		}
	})

	t.Run("reject — moderation_comment и уведомление", func(t *testing.T) {
		appID := insertApp()
		got, err := svc.RejectApplication(ctx, adm, appID, "уточните цель", service.ClientMeta{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Status != lending.AppStatusRejected || got.ModerationComment == nil || *got.ModerationComment != "уточните цель" {
			t.Fatalf("application = %+v", got)
		}
		if n := count(ctx, t, tx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'application_rejected'`, borrower.UserID); n != 1 {
			t.Fatalf("notifications = %d", n)
		}
	})
}

func TestBlockUserDeletesSessions(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	authSvc := auth.NewAuth(repository.New(tx), service.NewTxManager(tx), time.Hour,
		service.NewMemoryRateLimiter(), auth.DefaultAuthLimits)
	suffix := uuid.NewString()[:8]
	email := "block-" + suffix + "@example.test"
	const password = "blockPassword1"
	first, err := authSvc.Register(ctx, auth.RegisterInput{
		Email: email, Password: password, Role: "borrower",
		Org: auth.RegisterOrganizationInput{
			Name: "Block " + suffix, CountryCode: "RU", RegistrationNumber: servicetest.UniqueRegistrationNumber()[:12], LegalAddress: "Москва",
		},
	}, service.ClientMeta{UserAgent: "integration"})
	if err != nil {
		t.Fatalf("Register err = %v", err)
	}
	if _, err := authSvc.Login(ctx, email, password, service.ClientMeta{UserAgent: "integration"}); err != nil {
		t.Fatalf("Login err = %v", err)
	}
	userID := first.Me.User.ID
	sessions := func() int {
		return count(ctx, t, tx, `SELECT count(*) FROM sessions WHERE user_id = $1`, userID)
	}
	if n := sessions(); n != 2 {
		t.Fatalf("sessions before = %d, want 2", n)
	}

	adm := insertAdmin(ctx, t, tx)
	svc := newIntegrationAdmin(tx)
	blocked, err := svc.BlockUser(ctx, adm, userID, "мошенничество", service.ClientMeta{})
	if err != nil || blocked.Status != "blocked" {
		t.Fatalf("BlockUser = %+v, err = %v", blocked, err)
	}
	if n := sessions(); n != 0 {
		t.Fatalf("sessions after block = %d, want 0", n)
	}
	if _, err := authSvc.Authenticate(ctx, first.Token); apperr.CodeOf(err) != apperr.CodeUnauthorized {
		t.Fatalf("старая сессия: err = %v, want UNAUTHORIZED", err)
	}
	if _, err := authSvc.Login(ctx, email, password, service.ClientMeta{}); err == nil {
		t.Fatal("заблокированный вошёл")
	}

	// Повторная блокировка — без новой строки истории.
	if _, err := svc.BlockUser(ctx, adm, userID, "повторно", service.ClientMeta{}); err != nil {
		t.Fatalf("повторный BlockUser err = %v", err)
	}
	if n := count(ctx, t, tx, `SELECT count(*) FROM status_history WHERE entity_type = 'user' AND entity_id = $1`, userID); n != 1 {
		t.Fatalf("status_history = %d, want 1", n)
	}

	if _, err := svc.UnblockUser(ctx, adm, userID, service.ClientMeta{}); err != nil {
		t.Fatalf("UnblockUser err = %v", err)
	}
	if _, err := authSvc.Login(ctx, email, password, service.ClientMeta{}); err != nil {
		t.Fatalf("Login после unblock err = %v", err)
	}

	_, err = svc.BlockUser(ctx, adm, adm.UserID, "себя", service.ClientMeta{})
	servicetest.WantCode(t, err, apperr.CodeForbidden)
}

func TestAdminSearchIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	adm := insertAdmin(ctx, t, tx)
	svc := newIntegrationAdmin(tx)
	suffix := uuid.NewString()[:8]

	t.Run("q=a_b не находит axb", func(t *testing.T) {
		for _, local := range []string{"a_b", "axb"} {
			if _, err := tx.Exec(ctx, `INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', 'admin')`,
				"esc-"+suffix+"-"+local+"@example.test"); err != nil {
				t.Fatal(err)
			}
		}
		page, err := svc.ListUsers(ctx, adm, admin.UserFilter{Q: servicetest.Ptr(suffix + "-a_b")})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Email != "esc-"+suffix+"-a_b@example.test" {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("kyc_status — несколько значений через ИЛИ", func(t *testing.T) {
		for _, st := range []repository.KycStatus{repository.KycStatusUnderReview, repository.KycStatusRejected, repository.KycStatusDraft} {
			insertCompany(ctx, t, tx, "Queue "+suffix+" "+string(st), st)
		}
		page, err := svc.ListOrganizations(ctx, adm, admin.OrganizationFilter{
			KycStatuses: []string{"under_review", "rejected"}, Q: servicetest.Ptr("Queue " + suffix),
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if page.Total != 2 || len(page.Items) != 2 {
			t.Fatalf("total = %d, items = %d", page.Total, len(page.Items))
		}
		for _, o := range page.Items {
			if o.Org.KycStatus == "draft" || o.Beneficiaries == nil {
				t.Fatalf("org = %+v", o)
			}
		}
	})

	t.Run("карточка — пустые массивы и registry_check nil", func(t *testing.T) {
		company := insertCompany(ctx, t, tx, "Card "+suffix, repository.KycStatusUnderReview)
		insertFlag(ctx, t, tx, *company.OrgID, "high", "open", adm.UserID)
		d, err := svc.GetOrganization(ctx, adm, *company.OrgID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(d.Documents) != 0 || d.Documents == nil || len(d.Users) != 1 || len(d.AmlFlags) != 1 || d.RegistryCheck != nil {
			t.Fatalf("details = %+v", d)
		}
		if d.AmlFlags[0].Organization.Name != "Card "+suffix {
			t.Fatalf("flag organization = %+v", d.AmlFlags[0].Organization)
		}
	})
}

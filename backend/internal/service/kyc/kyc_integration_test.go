package kyc_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/storage"
)

// kycFixture — заёмщик в draft внутри внешней транзакции теста. Сервис открывает свои
// транзакции через tx.Begin (savepoint), так что всё откатывается в Cleanup.
type kycFixture struct {
	tx     pgx.Tx
	q      *repository.Queries
	kyc    *kyc.Kyc
	p      service.Principal
	orgID  uuid.UUID
	userID uuid.UUID
}

func newKycFixture(t *testing.T, deps kyc.KycDeps) *kycFixture {
	t.Helper()
	ctx := context.Background()
	tx := servicetest.BeginTx(t)

	f := &kycFixture{tx: tx, q: repository.New(tx)}
	err := tx.QueryRow(ctx, `INSERT INTO organizations (type, name, country_code, registration_number, legal_address)
		VALUES ('borrower', 'KYC Integration', 'RU', $1, 'Москва') RETURNING id`,
		servicetest.UniqueRegistrationNumber()).Scan(&f.orgID)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	orgID := f.orgID
	f.userID = servicetest.InsertUser(ctx, t, tx, repository.UserRoleBorrower, &orgID)
	f.p = service.Principal{UserID: f.userID, OrgID: &orgID, Role: "borrower", KycStatus: "draft"}

	deps.Store = f.q
	deps.Tx = service.NewTxManager(tx)
	f.kyc = kyc.NewKyc(deps)
	return f
}

// prepareForSubmit добавляет обязательные документы и бенефициара — всё, что submitKyc
// проверяет у заёмщика.
func (f *kycFixture) prepareForSubmit(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, dt := range requiredDocs {
		_, err := f.tx.Exec(ctx, `INSERT INTO kyc_documents (organization_id, doc_type, storage_key, file_name,
			mime_type, size_bytes, sha256, uploaded_by_user_id)
			VALUES ($1, $2, $3, 'doc.pdf', 'application/pdf', 10, sha256('x'), $4)`,
			f.orgID, dt, uuid.NewString(), f.userID)
		if err != nil {
			t.Fatalf("document %s: %v", dt, err)
		}
	}
	if _, err := f.tx.Exec(ctx, `INSERT INTO beneficiaries (organization_id, full_name, citizenship_country_code, ownership_percent)
		VALUES ($1, 'Иванов И.И.', 'RU', 100)`, f.orgID); err != nil {
		t.Fatalf("beneficiary: %v", err)
	}
}

// randomLEI — 20 символов [A-Z0-9]: LEI уникален, а демо-сид уже занимает свои.
func randomLEI() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, 20)
	_, _ = rand.Read(buf)
	var b strings.Builder
	for _, c := range buf {
		b.WriteByte(alphabet[int(c)%len(alphabet)])
	}
	return b.String()
}

func TestKycIntegration(t *testing.T) {
	dir := t.TempDir()
	files, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	f := newKycFixture(t, kyc.KycDeps{Files: files, Screener: aml.NoopScreener{}})
	ctx := context.Background()

	t.Run("загрузка PNG и удаление", func(t *testing.T) {
		png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{7}, 2048)...)
		doc, err := f.kyc.UploadDocument(ctx, f.p, service.ClientMeta{UserAgent: "it"},
			repository.KycDocumentTypeAddressProof, "адрес.png", bytes.NewReader(png))
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		docs, err := f.q.ListKycDocumentsByOrg(ctx, f.orgID)
		if err != nil || len(docs) != 1 {
			t.Fatalf("документы = %v, err %v", docs, err)
		}
		if docs[0].MimeType != "image/png" || len(docs[0].Sha256) != 32 || docs[0].SizeBytes != int64(len(png)) {
			t.Fatalf("строка = %+v", docs[0])
		}
		if _, err := os.Stat(filepath.Join(dir, doc.StorageKey)); err != nil {
			t.Fatalf("файла нет: %v", err)
		}

		if err := f.kyc.DeleteDocument(ctx, f.p, doc.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if docs, _ := f.q.ListKycDocumentsByOrg(ctx, f.orgID); len(docs) != 0 {
			t.Fatalf("строка осталась: %+v", docs)
		}
		if _, err := os.Stat(filepath.Join(dir, doc.StorageKey)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("файл остался: %v", err)
		}
	})

	t.Run("бенефициары: замена и очистка", func(t *testing.T) {
		_, err := f.kyc.UpdateMyOrganization(ctx, f.p, kyc.OrganizationUpdateInput{
			Beneficiaries: &[]kyc.BeneficiaryInput{beneficiary("Иванов И.И.", "60"), beneficiary("Петров П.П.", "40")},
		})
		if err != nil {
			t.Fatalf("PATCH: %v", err)
		}
		got, err := f.kyc.MyOrganization(ctx, f.p)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		if len(got.Beneficiaries) != 2 || got.Beneficiaries[0].FullName != "Иванов И.И." {
			t.Fatalf("бенефициары = %+v", got.Beneficiaries)
		}

		if _, err := f.kyc.UpdateMyOrganization(ctx, f.p, kyc.OrganizationUpdateInput{Beneficiaries: &[]kyc.BeneficiaryInput{}}); err != nil {
			t.Fatalf("PATCH []: %v", err)
		}
		got, _ = f.kyc.MyOrganization(ctx, f.p)
		if len(got.Beneficiaries) != 0 {
			t.Fatalf("после [] бенефициары = %+v", got.Beneficiaries)
		}
	})

	t.Run("lei: записать, не трогать, очистить", func(t *testing.T) {
		lei := randomLEI()
		got, err := f.kyc.UpdateMyOrganization(ctx, f.p, kyc.OrganizationUpdateInput{LEI: &lei})
		if err != nil {
			t.Fatalf("PATCH lei: %v", err)
		}
		if got.Org.LEI == nil || *got.Org.LEI != lei {
			t.Fatalf("lei = %v, want %s", got.Org.LEI, lei)
		}

		got, err = f.kyc.UpdateMyOrganization(ctx, f.p, kyc.OrganizationUpdateInput{Name: servicetest.Ptr("KYC Integration 2")})
		if err != nil {
			t.Fatalf("PATCH без lei: %v", err)
		}
		if got.Org.LEI == nil || *got.Org.LEI != lei {
			t.Fatalf("lei после PATCH без lei = %v, want %s", got.Org.LEI, lei)
		}

		got, err = f.kyc.UpdateMyOrganization(ctx, f.p, kyc.OrganizationUpdateInput{LEI: servicetest.Ptr("")})
		if err != nil {
			t.Fatalf(`PATCH lei "": %v`, err)
		}
		if got.Org.LEI != nil {
			t.Fatalf(`lei после "" = %q, want NULL`, *got.Org.LEI)
		}
	})
}

// count — скаляр COUNT(*) внутри транзакции теста.
func (f *kycFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestSubmitKycIntegration(t *testing.T) {
	// Одна находка — чтобы на реальной БД прошли HasSimilarAmlFlag и CreateAmlFlag.
	screener := &fakeScreener{findings: []aml.Finding{{
		Rule: repository.AmlRuleHighRiskJurisdiction, Severity: repository.AmlSeverityMedium, Details: "integration",
	}}}
	f := newKycFixture(t, kyc.KycDeps{Files: newFakeFiles(), Screener: screener})
	ctx := context.Background()

	servicetest.InsertUser(ctx, t, f.tx, repository.UserRoleAdmin, nil) // хотя бы один активный админ
	f.prepareForSubmit(t)
	admins := f.count(t, `SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active'`)
	notes := func(typ repository.NotificationType) int {
		return f.count(t, `SELECT count(*) FROM notifications WHERE type = $1 AND entity_id = $2`, typ, f.orgID.String())
	}
	flags := func() int { return f.count(t, `SELECT count(*) FROM aml_flags WHERE organization_id = $1`, f.orgID) }

	got, err := f.kyc.SubmitKyc(ctx, f.p, service.ClientMeta{UserAgent: "it"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got.Org.KycStatus != "under_review" {
		t.Fatalf("status = %s, want under_review", got.Org.KycStatus)
	}
	if n := f.count(t, `SELECT count(*) FROM status_history WHERE entity_type = 'organization' AND entity_id = $1
		AND from_status = 'draft' AND to_status = 'under_review' AND actor_user_id = $2`, f.orgID, f.userID); n != 1 {
		t.Fatalf("status_history = %d строк, want 1", n)
	}
	if n := notes(repository.NotificationTypeKycSubmitted); n != admins {
		t.Fatalf("kyc_submitted = %d, want по одному на %d активных админов", n, admins)
	}
	if n := notes(repository.NotificationTypeAmlFlagCreated); n != admins {
		t.Fatalf("aml_flag_created = %d, want %d", n, admins)
	}
	if n := flags(); n != 1 {
		t.Fatalf("aml_flags = %d, want 1", n)
	}

	t.Run("повторный submit из under_review — 409", func(t *testing.T) {
		_, err := f.kyc.SubmitKyc(ctx, f.p, service.ClientMeta{})
		servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
	})

	t.Run("после rejected — тот же флаг не дублируется", func(t *testing.T) {
		if _, err := f.tx.Exec(ctx, `UPDATE organizations SET kyc_status = 'rejected', kyc_reject_reason = 'нет устава'
			WHERE id = $1`, f.orgID); err != nil {
			t.Fatalf("reject: %v", err)
		}
		if _, err := f.kyc.SubmitKyc(ctx, f.p, service.ClientMeta{}); err != nil {
			t.Fatalf("resubmit: %v", err)
		}
		if n := flags(); n != 1 {
			t.Fatalf("aml_flags = %d, want 1 (дедупликация)", n)
		}
	})
}

// Снапшот мок-реестра при submitKyc (BE-06, шаг 7): одна строка registry_checks с
// organization_id компании и сырым ответом.
func TestSubmitKycRegistrySnapshotIntegration(t *testing.T) {
	f := newKycFixture(t, kyc.KycDeps{Files: newFakeFiles(), Screener: aml.NoopScreener{}, Verifier: registry.NewMock()})
	ctx := context.Background()

	// ИНН «Альфы» из мок-фикстур. Если демо-сид уже занял его, освобождаем номер внутри
	// транзакции теста — она откатывается в Cleanup.
	const alphaINN = "7700000009"
	if _, err := f.tx.Exec(ctx, `UPDATE organizations SET registration_number = registration_number || '-it'
		WHERE country_code = 'RU' AND registration_number = $1`, alphaINN); err != nil {
		t.Fatalf("free INN: %v", err)
	}
	if _, err := f.tx.Exec(ctx, `UPDATE organizations SET name = 'ООО «Альфа»', registration_number = $2 WHERE id = $1`,
		f.orgID, alphaINN); err != nil {
		t.Fatalf("set INN: %v", err)
	}
	servicetest.InsertUser(ctx, t, f.tx, repository.UserRoleAdmin, nil)
	f.prepareForSubmit(t)

	if _, err := f.kyc.SubmitKyc(ctx, f.p, service.ClientMeta{UserAgent: "it"}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	var (
		source string
		found  bool
		hasRaw bool
		rows   int
	)
	err := f.tx.QueryRow(ctx, `SELECT source::text, found, raw_response IS NOT NULL, count(*) OVER ()
		FROM registry_checks WHERE organization_id = $1`, f.orgID).Scan(&source, &found, &hasRaw, &rows)
	if err != nil {
		t.Fatalf("registry_checks: %v", err)
	}
	if rows != 1 || source != "mock" || !found || !hasRaw {
		t.Fatalf("строк %d, source %q, found %v, raw %v; want 1, mock, true, true", rows, source, found, hasRaw)
	}
}

// Настоящий скрининг (RuleScreener) на мок-списке: бенефициар из списка с гражданством IR
// даёт sanctions_match/high и high_risk_jurisdiction/high, одно уведомление на админа;
// повторная подача после rejected новых флагов не создаёт.
func TestSubmitKycAmlIntegration(t *testing.T) {
	ctx := context.Background()
	f := newKycFixture(t, kyc.KycDeps{Files: newFakeFiles(), Screener: aml.NoopScreener{}})
	f.kyc = kyc.NewKyc(kyc.KycDeps{Store: f.q, Tx: service.NewTxManager(f.tx), Files: newFakeFiles(), Screener: aml.RuleScreener{}})
	servicetest.RequireSanctionsMock(ctx, t, f.tx)
	servicetest.InsertUser(ctx, t, f.tx, repository.UserRoleAdmin, nil)
	f.prepareForSubmit(t)
	if _, err := f.tx.Exec(ctx, `UPDATE beneficiaries SET ownership_percent = 50 WHERE organization_id = $1`, f.orgID); err != nil {
		t.Fatalf("ownership: %v", err)
	}
	if _, err := f.tx.Exec(ctx, `INSERT INTO beneficiaries (organization_id, full_name, citizenship_country_code, ownership_percent)
		VALUES ($1, 'Золотарёв Кирилл', 'IR', 50)`, f.orgID); err != nil {
		t.Fatalf("beneficiary: %v", err)
	}
	admins := f.count(t, `SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active'`)
	flags := func() []string {
		rows, err := f.tx.Query(ctx, `SELECT rule || '/' || severity FROM aml_flags WHERE organization_id = $1 ORDER BY id`, f.orgID)
		if err != nil {
			t.Fatalf("flags: %v", err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("flags: %v", err)
		}
		return got
	}

	if _, err := f.kyc.SubmitKyc(ctx, f.p, service.ClientMeta{}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	want := []string{"high_risk_jurisdiction/high", "sanctions_match/high"}
	if got := flags(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("flags = %v, want %v", got, want)
	}
	if n := f.count(t, `SELECT count(*) FROM notifications WHERE type = 'aml_flag_created' AND entity_id = $1`, f.orgID.String()); n != admins {
		t.Fatalf("aml_flag_created = %d, want одно на каждого из %d админов", n, admins)
	}

	if _, err := f.tx.Exec(ctx, `UPDATE organizations SET kyc_status = 'rejected', kyc_reject_reason = 'нет устава'
		WHERE id = $1`, f.orgID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := f.kyc.SubmitKyc(ctx, f.p, service.ClientMeta{}); err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if got := flags(); len(got) != len(want) {
		t.Fatalf("после повторной подачи flags = %v, want без новых", got)
	}
}

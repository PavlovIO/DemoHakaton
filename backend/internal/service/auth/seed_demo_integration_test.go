package auth_test

import (
	"context"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

const (
	seedDir           = "../../../migrations/testdata/"
	mainApplicationID = "33333333-3333-3333-3333-333333333333"
	alphaUserID       = "aaaaaaaa-0000-0000-0000-000000000002"
)

// seedTables — таблицы, в которые пишет сид; повторный сид не должен менять число строк.
var seedTables = []string{
	"organizations", "beneficiaries", "users", "kyc_documents", "registry_checks", "applications",
	"offers", "deals", "messages", "notifications", "status_history", "aml_flags", "audit_log",
}

// Сид применяется в транзакции теста и откатывается: dev-БД не меняется. Дважды подряд —
// make seed-demo на уже заселённой БД (идемпотентность). На БД с нетронутым сидом B
// (make db) повторное применение B — тот же случай; сид A или прошедшее демо (оффер
// принят) меняют статусы, которые тест сверяет, — тогда пропуск.
func TestSeedDemoVariantB(t *testing.T) {
	ctx := context.Background()
	// Сид применяет суперпользователь (make seed-demo); lp_app не может TEMP VIEW из сида.
	tx := servicetest.BeginOwnerTx(t)
	if demoSeeded(ctx, t, tx) {
		var pristine bool
		err := tx.QueryRow(ctx, `SELECT
			(SELECT kyc_status = 'verified' FROM organizations WHERE id = '22222222-2222-2222-2222-222222222222')
			AND (SELECT status = 'published' FROM applications WHERE id = $1)
			AND (SELECT count(*) = 2 FROM offers WHERE application_id = $1 AND status = 'sent')`,
			mainApplicationID).Scan(&pristine)
		if err != nil {
			t.Fatal(err)
		}
		if !pristine {
			t.Skip("database holds variant A or a played demo: variant B needs make db-reset")
		}
	}
	applySeed(ctx, t, tx, "b")
	before := countRows(ctx, t, tx)
	applySeed(ctx, t, tx, "b")
	if after := countRows(ctx, t, tx); !maps.Equal(before, after) {
		t.Errorf("repeated seed changed row counts:\n before %v\n after  %v", before, after)
	}

	wantKycStatuses(ctx, t, tx, map[string]string{
		"alpha": "verified", "beta": "verified", "gamma": "under_review", "vector": "verified",
		"delta": "under_review", "epsilon": "verified", "omega": "rejected", "sigma": "verified",
	})
	wantShowcase(ctx, t, tx, 6, 2)
	wantOpenFlags(ctx, t, tx)

	t.Run("main application has offers from Beta and Epsilon", func(t *testing.T) {
		var lenders []string
		rows, err := tx.Query(ctx, `SELECT u.email FROM offers o JOIN users u ON u.organization_id = o.lender_organization_id
			WHERE o.application_id = $1 AND o.status = 'sent' ORDER BY u.email`, mainApplicationID)
		if err != nil {
			t.Fatal(err)
		}
		lenders, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(lenders, ",") != "beta@liquidity.local,epsilon@liquidity.local" {
			t.Errorf("sent offers from %v", lenders)
		}
	})

	t.Run("notification titles use the real application number", func(t *testing.T) {
		var bad int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM notifications n
			WHERE n.user_id = $1 AND n.title LIKE '%APP-%'
			  AND n.title NOT LIKE '%' || (SELECT 'APP-' || lpad(number::text, 6, '0') FROM applications WHERE id = $2) || '%'`,
			alphaUserID, mainApplicationID).Scan(&bad)
		if err != nil || bad != 0 {
			t.Errorf("notifications with a wrong APP number: %d, err %v", bad, err)
		}
	})

	t.Run("history for analytics", func(t *testing.T) {
		var applications, currencies, deals int
		err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM applications),
			(SELECT count(DISTINCT currency) FROM applications), (SELECT count(*) FROM deals)`).
			Scan(&applications, &currencies, &deals)
		if err != nil {
			t.Fatal(err)
		}
		if applications < 40 || currencies < 3 || deals < 10 {
			t.Errorf("applications %d (≥ 40), currencies %d (≥ 3), deals %d (≥ 10)", applications, currencies, deals)
		}
		var wrongFee int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM deals
			WHERE fee_percent IS NOT NULL AND fee_amount <> round(amount * fee_percent / 100, 2)`).Scan(&wrongFee)
		if err != nil || wrongFee != 0 {
			t.Errorf("deals with fee_amount ≠ amount × fee_percent / 100: %d, err %v", wrongFee, err)
		}
		// Создание → подача → публикация: иначе сортировка и аналитика по created_at
		// относили бы заявку не к тому периоду.
		var unordered int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM applications
			WHERE created_at > submitted_at OR submitted_at > published_at`).Scan(&unordered)
		if err != nil || unordered != 0 {
			t.Errorf("applications with created_at > submitted_at > published_at broken: %d, err %v", unordered, err)
		}
		// История — прошлое: дата в будущем исказила бы графики аналитики за период.
		var future int
		err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM applications WHERE greatest(created_at, submitted_at, published_at) > now())
			+ (SELECT count(*) FROM offers WHERE greatest(created_at, decided_at) > now())
			+ (SELECT count(*) FROM deals WHERE agreed_at > now())`).Scan(&future)
		if err != nil || future != 0 {
			t.Errorf("rows dated in the future: %d, err %v", future, err)
		}
	})

	wantKycTraces(ctx, t, tx)
	wantDemoLogins(ctx, t, tx)
}

// Вариант A и B несовместимы на одной БД (те же организации в разных статусах), поэтому
// на БД, где сид уже применён (make db), тест пропускается.
func TestSeedDemoVariantA(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginOwnerTx(t)
	if demoSeeded(ctx, t, tx) {
		t.Skip("demo seed is already applied: variant A needs a database without it (make db-reset SEED=a)")
	}
	applySeed(ctx, t, tx, "a")
	before := countRows(ctx, t, tx)
	applySeed(ctx, t, tx, "a")
	if after := countRows(ctx, t, tx); !maps.Equal(before, after) {
		t.Errorf("repeated seed changed row counts:\n before %v\n after  %v", before, after)
	}

	wantKycStatuses(ctx, t, tx, map[string]string{
		"alpha": "draft", "beta": "under_review", "gamma": "under_review", "vector": "verified",
		"delta": "under_review", "epsilon": "verified", "omega": "rejected", "sigma": "verified",
	})
	wantShowcase(ctx, t, tx, 5, 1)
	wantOpenFlags(ctx, t, tx)
	var betaDocs int
	err := tx.QueryRow(ctx, `SELECT count(DISTINCT doc_type) FROM kyc_documents
		WHERE organization_id = '22222222-2222-2222-2222-222222222222'`).Scan(&betaDocs)
	if err != nil || betaDocs != 5 {
		t.Errorf("Beta KYC document types = %d, want 5 (err %v)", betaDocs, err)
	}
	wantKycTraces(ctx, t, tx)
	wantDemoLogins(ctx, t, tx)
}

func demoSeeded(ctx context.Context, t *testing.T, tx pgx.Tx) bool {
	t.Helper()
	var seeded bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, alphaUserID).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	return seeded
}

// applySeed выполняет seed_demo_common.sql, файл варианта и seed_demo_kyc.sql, как
// make seed-demo (psql --single-transaction), но в транзакции теста.
func applySeed(ctx context.Context, t *testing.T, tx pgx.Tx, variant string) {
	t.Helper()
	for _, file := range []string{"seed_demo_common.sql", "seed_demo_" + variant + ".sql", "seed_demo_kyc.sql"} {
		sql, err := os.ReadFile(seedDir + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
}

func countRows(ctx context.Context, t *testing.T, tx pgx.Tx) map[string]int {
	t.Helper()
	counts := make(map[string]int, len(seedTables))
	for _, table := range seedTables {
		var n int
		// Имя таблицы — из константного списка выше, не из ввода.
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

// wantKycStatuses — статусы KYC по префиксу email пользователя компании (таблица шага 1).
func wantKycStatuses(ctx context.Context, t *testing.T, tx pgx.Tx, want map[string]string) {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT split_part(u.email::text, '@', 1), o.kyc_status::text
		FROM organizations o JOIN users u ON u.organization_id = o.id WHERE u.email LIKE '%@liquidity.local'`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for rows.Next() {
		var login, status string
		if err := rows.Scan(&login, &status); err != nil {
			t.Fatal(err)
		}
		got[login] = status
	}
	if !maps.Equal(got, want) {
		t.Errorf("kyc statuses = %v, want %v", got, want)
	}
}

// wantShowcase — витрина демо-компаний: опубликованных заявок всего и с целевой страной DE
// (фильтр «Германия» из US-14). Заявки других интеграционных тестов (часть из них
// коммитит фикстуры) не считаются.
func wantShowcase(ctx context.Context, t *testing.T, tx pgx.Tx, wantTotal, wantDE int) {
	t.Helper()
	var total, de int
	err := tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE a.target_country_code = 'DE')
		FROM applications a JOIN users u ON u.organization_id = a.borrower_organization_id
		WHERE a.status = 'published' AND u.email LIKE '%@liquidity.local'`).Scan(&total, &de)
	if err != nil {
		t.Fatal(err)
	}
	if total != wantTotal || de != wantDE {
		t.Errorf("published applications = %d (DE %d), want %d (DE %d)", total, de, wantTotal, wantDE)
	}
}

// wantOpenFlags — открытые флаги демо-компаний: Gamma high (санкции), Delta medium (FATF).
// Флаги других тестов и ручных проверок не считаются.
func wantOpenFlags(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT o.name || ':' || f.rule || '/' || f.severity FROM aml_flags f
		JOIN organizations o ON o.id = f.organization_id
		WHERE f.status = 'open'
		  AND EXISTS (SELECT 1 FROM users u WHERE u.organization_id = o.id AND u.email LIKE '%@liquidity.local')
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	flags, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	want := "Delta Trading:high_risk_jurisdiction/medium,Gamma GmbH:sanctions_match/high"
	if strings.Join(flags, ",") != want {
		t.Errorf("open aml flags = %v, want %s", flags, want)
	}
}

// wantKycTraces — у каждой демо-компании, подавшей KYC, есть то, что пишут submitKyc и
// решение админа: история, аудит и уведомления (админу о подаче, компании о решении).
func wantKycTraces(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT o.name FROM organizations o
		JOIN users u ON u.organization_id = o.id
		WHERE u.email LIKE '%@liquidity.local' AND o.kyc_submitted_at IS NOT NULL
		  AND (NOT EXISTS (SELECT 1 FROM status_history h WHERE h.entity_type = 'organization'
		                   AND h.entity_id = o.id AND h.to_status = o.kyc_status::text)
		    OR NOT EXISTS (SELECT 1 FROM audit_log a WHERE a.action = 'kyc.submit' AND a.entity_id = o.id::text)
		    OR NOT EXISTS (SELECT 1 FROM notifications n WHERE n.type = 'kyc_submitted' AND n.entity_id = o.id::text)
		    OR (o.kyc_status IN ('verified', 'rejected') AND (
		        NOT EXISTS (SELECT 1 FROM audit_log a WHERE a.action IN ('kyc.approve', 'kyc.reject')
		                    AND a.entity_id = o.id::text)
		     OR NOT EXISTS (SELECT 1 FROM notifications n WHERE n.user_id = u.id
		                    AND n.type IN ('kyc_approved', 'kyc_rejected') AND n.entity_id = o.id::text))))
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("companies without KYC history/audit/notifications: %v", missing)
	}
}

// wantDemoLogins — вход общим паролем из README: активный аккаунт входит, «Сигма»
// заблокирована, неверный пароль — 401, а не 500 (как было с заглушками хэшей).
func wantDemoLogins(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	svc := auth.NewAuth(repository.New(tx), service.NewTxManager(tx), time.Hour,
		service.NewMemoryRateLimiter(), auth.DefaultAuthLimits)
	meta := service.ClientMeta{UserAgent: "seed-integration"}
	for _, email := range []string{"admin@liquidity.local", "alpha@liquidity.local", "beta@liquidity.local"} {
		if _, err := svc.Login(ctx, email, demoPassword, meta); err != nil {
			t.Errorf("Login(%s) err = %v", email, err)
		}
	}
	if _, err := svc.Login(ctx, "sigma@liquidity.local", demoPassword, meta); apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Errorf("Login(sigma) err = %v, want FORBIDDEN", err)
	}
	if _, err := svc.Login(ctx, "gamma@liquidity.local", "wrong-password-1", meta); apperr.CodeOf(err) != apperr.CodeUnauthorized {
		t.Errorf("Login(gamma, wrong password) err = %v, want UNAUTHORIZED", err)
	}
}

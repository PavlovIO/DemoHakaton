package aml_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// Результат SearchSanctions не зависит от GUC pg_trgm.similarity_threshold, пока GUC не
// выше порога в запросе (0.4 = aml.candidateMinSimilarity): 0.3 — дефолт до DB-SEC, 0.4 —
// значение на роли lp_app после.
func TestSearchSanctionsThresholdIndependent(t *testing.T) {
	ctx := context.Background()
	// У «zolotarev» есть кандидат «kirill zolotaryov» с similarity 0.333: GUC 0.3 его
	// пропускает через «%», явный порог — нет.
	query := aml.Normalize("Zolotarev")
	search := func(guc string) []int64 {
		tx := servicetest.BeginTx(t)
		servicetest.RequireSanctionsMock(ctx, t, tx)
		if _, err := tx.Exec(ctx, "SET LOCAL pg_trgm.similarity_threshold = "+guc); err != nil {
			t.Fatalf("set guc: %v", err)
		}
		rows, err := repository.New(tx).SearchSanctions(ctx, repository.SearchSanctionsParams{
			Query: query, MinSimilarity: 0.4, EntityKinds: []string{"person", "other"}, MaxCandidates: 20,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		ids := make([]int64, len(rows))
		for i, r := range rows {
			if r.TrgmScore < 0.4 {
				t.Errorf("GUC %s: кандидат %q со score %.2f < 0.4", guc, r.NameNormalized, r.TrgmScore)
			}
			ids[i] = r.ID
		}
		return ids
	}

	low, high := search("0.3"), search("0.4")
	if len(low) == 0 {
		t.Fatal("кандидатов нет: в мок-списке есть M-001 «Zolotarev Kirill»")
	}
	if !slices.Equal(low, high) {
		t.Fatalf("GUC 0.3 → %v, GUC 0.4 → %v", low, high)
	}
}

// Фильтр entity_kind стоит до LIMIT: 25 персон, похожих на название компании по триграммам
// сильнее организации, не вытесняют её из кандидатов, и компания получает флаг.
func TestScreenOrganizationKindBeforeLimit(t *testing.T) {
	ctx := context.Background()
	// Порог «%» берём с настоящей роли приложения: у владельца схемы дефолт 0.3, и тест
	// не заметил бы, что значение на lp_app (backend/db/roles.sql) отсекает организацию.
	var appThreshold string
	if err := servicetest.NewPool(t).QueryRow(ctx, "SHOW pg_trgm.similarity_threshold").Scan(&appThreshold); err != nil {
		t.Fatalf("lp_app threshold: %v", err)
	}
	// Списки пишет загрузчик под владельцем схемы, у lp_app на sanctions_entries только SELECT.
	tx := servicetest.BeginOwnerTx(t)
	if _, err := tx.Exec(ctx, "SELECT set_config('pg_trgm.similarity_threshold', $1, true)", appThreshold); err != nil {
		t.Fatalf("set threshold: %v", err)
	}
	insert := func(id, kind, name string) {
		t.Helper()
		if _, err := tx.Exec(ctx, `INSERT INTO sanctions_entries (source, external_id, entity_kind, name, name_normalized)
			VALUES ('test', $1, $2, $3, $4)`, id, kind, name, aml.Normalize(name)); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	for i := range 25 {
		insert(fmt.Sprintf("P-%02d", i), "person", fmt.Sprintf("Quorvex Trading %c", 'a'+i))
	}
	insert("O-1", "organization", "Quorvex Tradinq Ltd")

	got, err := aml.RuleScreener{}.ScreenOrganization(ctx, repository.New(tx), aml.OrganizationSubject{
		Name: "Quorvex Trading", CountryCode: "RU",
	})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if len(got) != 1 || got[0].Rule != repository.AmlRuleSanctionsMatch || !strings.Contains(got[0].Details, "O-1") {
		t.Fatalf("находки = %+v, нужна sanctions_match по O-1", got)
	}
}

package aml_test

import (
	"context"
	"slices"
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
			Query: query, MinSimilarity: 0.4, MaxCandidates: 20,
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

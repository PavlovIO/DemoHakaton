package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeFlagStore считает дублем находку с details из existing.
type fakeFlagStore struct {
	repository.Querier
	existing map[string]bool
	dedupArg []repository.HasSimilarAmlFlagParams
	created  []repository.CreateAmlFlagParams
}

func (s *fakeFlagStore) HasSimilarAmlFlag(_ context.Context, arg repository.HasSimilarAmlFlagParams) (bool, error) {
	s.dedupArg = append(s.dedupArg, arg)
	return s.existing[arg.Details], nil
}

func (s *fakeFlagStore) CreateAmlFlag(_ context.Context, arg repository.CreateAmlFlagParams) (repository.AmlFlag, error) {
	s.created = append(s.created, arg)
	return repository.AmlFlag{ID: int64(len(s.created)), Details: arg.Details}, nil
}

func TestSaveAmlFindings(t *testing.T) {
	orgID, appID := uuid.New(), uuid.New()
	store := &fakeFlagStore{existing: map[string]bool{"уже открыт": true}}
	findings := []aml.Finding{
		{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh, Details: "уже открыт"},
		{Rule: repository.AmlRuleAmountThreshold, Severity: repository.AmlSeverityMedium, Details: "новый"},
	}

	got, err := service.SaveAmlFindings(context.Background(), store, orgID, &appID, findings)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || got[0].Details != "новый" {
		t.Fatalf("created flags = %+v, want только «новый»", got)
	}
	if len(store.created) != 1 || store.created[0].OrganizationID != orgID || store.created[0].ApplicationID.Bytes != appID ||
		store.created[0].Rule != repository.AmlRuleAmountThreshold {
		t.Fatalf("CreateAmlFlag = %+v", store.created)
	}
	if len(store.dedupArg) != 2 || store.dedupArg[0].Rule != repository.AmlRuleSanctionsMatch || !store.dedupArg[0].ApplicationID.Valid {
		t.Fatalf("HasSimilarAmlFlag = %+v", store.dedupArg)
	}
}

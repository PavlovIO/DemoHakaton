package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// submitStore — fakeKycStore плюс запросы submitKyc.
type submitStore struct {
	*fakeKycStore
	submitted     bool
	history       []repository.CreateStatusHistoryParams
	flags         []repository.CreateAmlFlagParams
	notifications []repository.CreateNotificationsForAdminsParams
	registryRows  []repository.CreateRegistryCheckParams
}

func (s *submitStore) ListKycDocumentsByOrg(_ context.Context, orgID uuid.UUID) ([]repository.KycDocument, error) {
	var out []repository.KycDocument
	for _, d := range s.documents {
		if d.OrganizationID == orgID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *submitStore) SubmitKycForReview(_ context.Context, id uuid.UUID) (repository.Organization, error) {
	s.submitted = true
	s.org.KycStatus = repository.KycStatusUnderReview
	s.org.KycRejectReason = pgtype.Text{}
	return s.org, nil
}

func (s *submitStore) CreateStatusHistory(_ context.Context, arg repository.CreateStatusHistoryParams) (repository.StatusHistory, error) {
	s.history = append(s.history, arg)
	return repository.StatusHistory{}, nil
}

func (s *submitStore) HasSimilarAmlFlag(context.Context, repository.HasSimilarAmlFlagParams) (bool, error) {
	return false, nil
}

func (s *submitStore) CreateAmlFlag(_ context.Context, arg repository.CreateAmlFlagParams) (repository.AmlFlag, error) {
	s.flags = append(s.flags, arg)
	return repository.AmlFlag{ID: int64(len(s.flags)), OrganizationID: arg.OrganizationID, Rule: arg.Rule}, nil
}

func (s *submitStore) CreateNotificationsForAdmins(_ context.Context, arg repository.CreateNotificationsForAdminsParams) (int64, error) {
	s.notifications = append(s.notifications, arg)
	return 1, nil
}

func (s *submitStore) CreateRegistryCheck(_ context.Context, arg repository.CreateRegistryCheckParams) (repository.RegistryCheck, error) {
	s.registryRows = append(s.registryRows, arg)
	return repository.RegistryCheck{}, nil
}

type submitTx struct{ store *submitStore }

func (t submitTx) InTx(_ context.Context, fn func(q repository.Querier) error) error {
	return fn(t.store)
}

type fakeScreener struct {
	findings []aml.Finding
	err      error
	called   bool
	subject  aml.OrganizationSubject
}

func (f *fakeScreener) ScreenOrganization(_ context.Context, s aml.OrganizationSubject) ([]aml.Finding, error) {
	f.called, f.subject = true, s
	return f.findings, f.err
}

type fakeVerifier struct {
	result registry.Result
	err    error
}

func (f fakeVerifier) Lookup(context.Context, string, string) (registry.Result, error) {
	return f.result, f.err
}

var requiredDocs = []repository.KycDocumentType{
	repository.KycDocumentTypeCharter, repository.KycDocumentTypeRegistrationCertificate,
	repository.KycDocumentTypeAddressProof, repository.KycDocumentTypeBeneficiariesRegister,
}

func TestSubmitKyc(t *testing.T) {
	type setup struct {
		status        repository.KycStatus
		lender        bool
		docs          []repository.KycDocumentType
		noBeneficiary bool
		screener      fakeScreener
		verifier      service.CompanyVerifier
	}
	without := func(skip ...repository.KycDocumentType) []repository.KycDocumentType {
		var out []repository.KycDocumentType
		for _, d := range requiredDocs {
			keep := true
			for _, s := range skip {
				keep = keep && d != s
			}
			if keep {
				out = append(out, d)
			}
		}
		return out
	}
	run := func(t *testing.T, su setup) (*submitStore, *fakeScreener, service.OrganizationProfile, error) {
		t.Helper()
		store := &submitStore{fakeKycStore: newFakeKycStore(su.status)}
		if su.lender {
			store.org.Type, store.org.CountryCode = repository.OrganizationTypeLender, "CY"
		}
		if su.status == repository.KycStatusRejected {
			store.org.KycRejectReason = pgtype.Text{String: "нет устава", Valid: true}
		}
		for _, d := range su.docs {
			store.documents = append(store.documents, repository.KycDocument{ID: uuid.New(), OrganizationID: store.org.ID, DocType: d})
		}
		if !su.noBeneficiary {
			store.beneficiaries = []repository.Beneficiary{{ID: uuid.New(), OrganizationID: store.org.ID, FullName: "Иванов И.И.", CitizenshipCountryCode: "RU"}}
		}
		screener := su.screener
		kyc := service.NewKyc(service.KycDeps{
			Store: store, Tx: submitTx{store: store}, Files: newFakeFiles(),
			Screener: &screener, Verifier: su.verifier,
		})
		profile, err := kyc.SubmitKyc(context.Background(), store.principal(), service.ClientMeta{})
		return store, &screener, profile, err
	}

	t.Run("draft, 4 документа — under_review, история, уведомление, аудит", func(t *testing.T) {
		store, screener, profile, err := run(t, setup{status: repository.KycStatusDraft, docs: requiredDocs})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if profile.Org.KycStatus != "under_review" || !screener.called {
			t.Fatalf("status = %s, скрининг = %v", profile.Org.KycStatus, screener.called)
		}
		h := store.history
		if len(h) != 1 || h[0].EntityType != repository.EntityTypeOrganization || h[0].FromStatus.String != "draft" ||
			h[0].ToStatus != "under_review" || h[0].ActorRole.UserRole != repository.UserRoleBorrower || !h[0].ActorUserID.Valid {
			t.Fatalf("status_history = %+v", h)
		}
		if len(store.notifications) != 1 || store.notifications[0].Type != repository.NotificationTypeKycSubmitted {
			t.Fatalf("notifications = %+v", store.notifications)
		}
		if len(store.audit) != 1 || store.audit[0].Action != "kyc.submit" {
			t.Fatalf("audit = %+v", store.audit)
		}
		if screener.subject.Registry != nil {
			t.Fatalf("Registry = %+v, want nil без verifier", screener.subject.Registry)
		}
	})

	t.Run("без бенефициаров — 400, скрининг не вызван", func(t *testing.T) {
		store, screener, _, err := run(t, setup{status: repository.KycStatusDraft, docs: requiredDocs, noBeneficiary: true})
		wantValidationField(t, err, "beneficiaries")
		if screener.called || store.submitted {
			t.Fatal("скрининг или переход выполнены")
		}
	})

	t.Run("lender без source_of_funds — 400", func(t *testing.T) {
		_, _, _, err := run(t, setup{status: repository.KycStatusDraft, lender: true, docs: requiredDocs})
		wantDocsMissing(t, err, "source_of_funds")
	})

	t.Run("нет charter и address_proof — две записи", func(t *testing.T) {
		_, _, _, err := run(t, setup{status: repository.KycStatusDraft,
			docs: without(repository.KycDocumentTypeCharter, repository.KycDocumentTypeAddressProof)})
		wantDocsMissing(t, err, "charter", "address_proof")
	})

	for _, st := range []repository.KycStatus{repository.KycStatusUnderReview, repository.KycStatusVerified} {
		t.Run(string(st)+" — 409", func(t *testing.T) {
			store, _, _, err := run(t, setup{status: st, docs: requiredDocs})
			wantCode(t, err, apperr.CodeInvalidTransition)
			if store.submitted {
				t.Fatal("переход выполнен")
			}
		})
	}

	t.Run("rejected — 200, причина отказа сброшена", func(t *testing.T) {
		_, _, profile, err := run(t, setup{status: repository.KycStatusRejected, docs: requiredDocs})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if profile.Org.KycStatus != "under_review" || profile.Org.KycRejectReason != nil {
			t.Fatalf("org = %+v", profile.Org)
		}
	})

	t.Run("скрининг упал — ошибка, перехода нет", func(t *testing.T) {
		store, _, _, err := run(t, setup{status: repository.KycStatusDraft, docs: requiredDocs,
			screener: fakeScreener{err: errors.New("sanctions table locked")}})
		if err == nil {
			t.Fatal("want error")
		}
		if _, ok := apperr.As(err); ok {
			t.Fatalf("err = %v, want внутреннюю (500)", err)
		}
		if store.submitted {
			t.Fatal("SubmitKycForReview вызван")
		}
	})

	t.Run("2 находки — 2 флага и одно aml_flag_created", func(t *testing.T) {
		store, _, _, err := run(t, setup{status: repository.KycStatusDraft, docs: requiredDocs,
			screener: fakeScreener{findings: []aml.Finding{
				{Rule: repository.AmlRuleSanctionsMatch, Severity: repository.AmlSeverityHigh, Details: "a"},
				{Rule: repository.AmlRuleHighRiskJurisdiction, Severity: repository.AmlSeverityMedium, Details: "b"},
			}}})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(store.flags) != 2 {
			t.Fatalf("flags = %+v", store.flags)
		}
		var amlNotes int
		for _, n := range store.notifications {
			if n.Type == repository.NotificationTypeAmlFlagCreated {
				amlNotes++
			}
		}
		if amlNotes != 1 {
			t.Fatalf("aml_flag_created = %d, want 1", amlNotes)
		}
	})

	t.Run("реестр недоступен — 200, Outcome unavailable в скрининге", func(t *testing.T) {
		store, screener, _, err := run(t, setup{status: repository.KycStatusDraft, docs: requiredDocs,
			verifier: fakeVerifier{err: registry.ErrUnavailable}})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if screener.subject.Registry == nil || screener.subject.Registry.Outcome != aml.RegistryUnavailable {
			t.Fatalf("Registry = %+v", screener.subject.Registry)
		}
		if len(store.registryRows) != 0 {
			t.Fatalf("снапшот без ответа реестра: %+v", store.registryRows)
		}
	})

	t.Run("реестр ответил — снапшот в registry_checks", func(t *testing.T) {
		store, screener, _, err := run(t, setup{status: repository.KycStatusDraft, docs: requiredDocs,
			verifier: fakeVerifier{result: registry.Result{Source: registry.SourceMock, Query: "7707083893", Found: true, Name: "ООО Альфа", Status: "active"}}})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if screener.subject.Registry == nil || screener.subject.Registry.Outcome != aml.RegistryChecked || !screener.subject.Registry.Found {
			t.Fatalf("Registry = %+v", screener.subject.Registry)
		}
		if len(store.registryRows) != 1 || store.registryRows[0].OrganizationID.Bytes != store.org.ID {
			t.Fatalf("registry_checks = %+v", store.registryRows)
		}
	})
}

// wantDocsMissing — 400 с записью field=documents на каждый недостающий тип.
func wantDocsMissing(t *testing.T, err error, docTypes ...string) {
	t.Helper()
	wantCode(t, err, apperr.CodeValidation)
	e, _ := apperr.As(err)
	for _, dt := range docTypes {
		found := false
		for _, d := range e.Details {
			found = found || (d.Field == "documents" && d.Message == "нет документа: "+dt)
		}
		if !found {
			t.Errorf("details = %+v, want «нет документа: %s»", e.Details, dt)
		}
	}
	if len(e.Details) != len(docTypes) {
		t.Errorf("details = %d записей, want %d", len(e.Details), len(docTypes))
	}
}

func TestNewKycRequiresScreener(t *testing.T) {
	// Без скринера submitKyc не может пройти AML; ошибка конфигурации должна всплыть при
	// старте, а не паникой на первом submit.
	defer func() {
		if recover() == nil {
			t.Fatal("NewKyc без Screener не паникует")
		}
	}()
	store := newFakeKycStore(repository.KycStatusDraft)
	service.NewKyc(service.KycDeps{Store: store, Tx: fakeKycTx{store: store}, Files: newFakeFiles()})
}

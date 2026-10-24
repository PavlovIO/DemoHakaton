package kyc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// countryRU — для RU реестр ищет по ИНН, для остальных — по LEI.
const countryRU = "RU"

// requiredKycDocuments — без них компанию нельзя отправить на проверку (описание submitKyc).
var requiredKycDocuments = []repository.KycDocumentType{
	repository.KycDocumentTypeCharter,
	repository.KycDocumentTypeRegistrationCertificate,
	repository.KycDocumentTypeAddressProof,
	repository.KycDocumentTypeBeneficiariesRegister,
}

// requiredLenderDocuments — кредитору дополнительно: происхождение средств.
var requiredLenderDocuments = []repository.KycDocumentType{repository.KycDocumentTypeSourceOfFunds}

// OrgScreener — AML-скрининг компании (aml.Screener).
type OrgScreener interface {
	ScreenOrganization(ctx context.Context, store aml.Store, s aml.OrganizationSubject) ([]aml.Finding, error)
}

// CompanyVerifier — сверка с реестром (registry.CompanyVerifier, BE-06).
type CompanyVerifier interface {
	Lookup(ctx context.Context, countryCode, number string) (registry.Result, error)
}

// SubmitKyc отправляет компанию на проверку: draft|rejected → under_review.
//
// До транзакции — ранний 409 и сверка с реестром: сетевой вызов не держит блокировку
// строки (idle_in_transaction_session_timeout = 15 с у lp_app). В транзакции под
// блокировкой: статус, неизменность реквизитов после сверки, обязательные документы,
// бенефициары, AML-скрининг, переход, история, снапшот реестра, флаги, аудит и
// уведомления — всё или ничего. Скрининг упал — submit падает целиком (fail-closed):
// без AML в under_review не переводим.
func (k *Kyc) SubmitKyc(ctx context.Context, p service.Principal, meta service.ClientMeta) (OrganizationProfile, error) {
	orgID, err := ownOrganization(p)
	if err != nil {
		return OrganizationProfile{}, err
	}
	org, err := k.store.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return OrganizationProfile{}, pgerr.Map(err, "Организация не найдена")
	}
	if _, err := NextKycStatus(org.KycStatus, KycSubmit); err != nil {
		return OrganizationProfile{}, err
	}
	registryCheck, snapshot := k.checkRegistry(ctx, org)
	checked := org

	err = k.tx.InTx(ctx, func(q repository.Querier) error {
		org, err := q.GetOrganizationForUpdate(ctx, orgID)
		if err != nil {
			return pgerr.Map(err, "Организация не найдена")
		}
		// Реестр сверялся до блокировки: параллельный PATCH мог сменить реквизиты, и тогда
		// снапшот и AML-сверка относились бы к другой компании.
		if registryCheck != nil && !sameRegistryIdentity(checked, org) {
			return apperr.Conflict("Реквизиты компании изменились во время проверки — отправьте повторно")
		}
		to, err := NextKycStatus(org.KycStatus, KycSubmit)
		if err != nil {
			return err
		}
		if err := checkRequiredDocuments(ctx, q, org); err != nil {
			return err
		}
		findings, err := k.screen(ctx, q, org, registryCheck)
		if err != nil {
			return err
		}
		if _, err := q.SubmitKycForReview(ctx, orgID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) { // охрана статуса в WHERE
				return apperr.InvalidTransition(string(org.KycStatus), string(to))
			}
			return fmt.Errorf("submit kyc: %w", err)
		}
		return k.recordSubmit(ctx, q, p, meta, org, to, snapshot, findings)
	})
	if err != nil {
		return OrganizationProfile{}, err
	}
	return LoadProfile(ctx, k.store, orgID)
}

// checkRegistry сверяет компанию с реестром, если verifier подключён (BE-06). Сбой
// реестра не блокирует submit — «мягкая деградация» (issue #22): AML получит Outcome
// unavailable, и админ сверит вручную. snapshot != nil — ответ реестра для registry_checks.
func (k *Kyc) checkRegistry(ctx context.Context, org repository.Organization) (*aml.RegistryResult, *registry.Result) {
	if k.verifier == nil {
		return nil, nil
	}
	number := org.RegistrationNumber
	if org.CountryCode != countryRU {
		if !org.Lei.Valid {
			return &aml.RegistryResult{Outcome: aml.RegistryNoIdentifier}, nil
		}
		number = org.Lei.String
	}
	result, err := k.verifier.Lookup(ctx, org.CountryCode, number)
	if err != nil {
		// Любой сбой, а не только ErrUnavailable: битый ответ или баг адаптера не должны
		// превращать внешний реестр в 500 на submit — админ сверит вручную.
		slog.WarnContext(ctx, "registry lookup failed on kyc submit", "organization_id", org.ID, "err", err)
		return amlRegistryResult(result, err), nil
	}
	return amlRegistryResult(result, nil), &result
}

// sameRegistryIdentity — те же идентификаторы, по которым checkRegistry ищет в реестре.
func sameRegistryIdentity(a, b repository.Organization) bool {
	return a.CountryCode == b.CountryCode && a.RegistrationNumber == b.RegistrationNumber && a.Lei == b.Lei
}

// checkRequiredDocuments — 400 с записью на каждый недостающий тип.
func checkRequiredDocuments(ctx context.Context, q repository.Querier, org repository.Organization) error {
	docs, err := q.ListKycDocumentsByOrg(ctx, org.ID)
	if err != nil {
		return fmt.Errorf("list kyc documents: %w", err)
	}
	have := make(map[repository.KycDocumentType]bool, len(docs))
	for _, d := range docs {
		have[d.DocType] = true
	}
	required := requiredKycDocuments
	if org.Type == repository.OrganizationTypeLender {
		required = append(append([]repository.KycDocumentType{}, required...), requiredLenderDocuments...)
	}
	var missing []apperr.FieldError
	for _, dt := range required {
		if !have[dt] {
			missing = append(missing, apperr.FieldError{Field: "documents", Message: "нет документа: " + string(dt)})
		}
	}
	if len(missing) > 0 {
		return apperr.Validation("Загружены не все обязательные документы", missing...)
	}
	return nil
}

// screen — AML-скрининг компании с бенефициарами. Без бенефициаров скринить нечего,
// кроме самой компании, поэтому нужен хотя бы один (DECISIONS п. 8).
func (k *Kyc) screen(ctx context.Context, q repository.Querier, org repository.Organization, reg *aml.RegistryResult) ([]aml.Finding, error) {
	rows, err := q.ListBeneficiariesByOrg(ctx, org.ID)
	if err != nil {
		return nil, fmt.Errorf("list beneficiaries: %w", err)
	}
	if len(rows) == 0 {
		return nil, apperr.Validation("Укажите хотя бы одного бенефициара", apperr.FieldError{
			Field: "beneficiaries", Message: "нужен хотя бы один бенефициар",
		})
	}
	persons := make([]aml.Person, len(rows))
	for i, b := range rows {
		persons[i] = aml.Person{FullName: b.FullName, CitizenshipCountryCode: b.CitizenshipCountryCode}
	}
	findings, err := k.screener.ScreenOrganization(ctx, q, aml.OrganizationSubject{
		Name: org.Name, CountryCode: org.CountryCode, Beneficiaries: persons, Registry: reg,
	})
	if err != nil {
		return nil, fmt.Errorf("aml screening: %w", err)
	}
	return findings, nil
}

// recordSubmit — следы перехода в той же транзакции: история, снапшот реестра, флаги,
// аудит и уведомления админам (одно aml_flag_created на весь скрининг — DECISIONS п. 14).
func (k *Kyc) recordSubmit(ctx context.Context, q repository.Querier, p service.Principal, meta service.ClientMeta,
	org repository.Organization, to repository.KycStatus, snapshot *registry.Result, findings []aml.Finding,
) error {
	if err := service.RecordStatusChange(ctx, q, service.StatusChange{
		EntityType: repository.EntityTypeOrganization,
		EntityID:   org.ID,
		From:       string(org.KycStatus),
		To:         string(to),
		Actor:      &p,
	}); err != nil {
		return err
	}
	if snapshot != nil {
		if _, err := q.CreateRegistryCheck(ctx, registryCheckParams(org.ID, *snapshot)); err != nil {
			return fmt.Errorf("save registry check: %w", err)
		}
	}
	flags, err := service.SaveAmlFindings(ctx, q, org.ID, nil, findings)
	if err != nil {
		return err
	}
	if err := service.WriteAudit(ctx, q, meta, service.AuditEvent{
		Action: AuditKycSubmit, UserID: &p.UserID,
		EntityType: repository.EntityTypeOrganization, EntityID: org.ID.String(),
	}); err != nil {
		return err
	}
	if err := k.notifier.NotifyAdmins(ctx, q, organizationNotification(
		repository.NotificationTypeKycSubmitted, "Компания отправлена на проверку KYC: "+org.Name, org.ID)); err != nil {
		return err
	}
	if len(flags) == 0 {
		return nil
	}
	return k.notifier.NotifyAdmins(ctx, q, organizationNotification(
		repository.NotificationTypeAmlFlagCreated, fmt.Sprintf("AML: %d новых флагов — %s", len(flags), org.Name), org.ID))
}

func organizationNotification(t repository.NotificationType, title string, orgID uuid.UUID) service.Notification {
	return service.Notification{Type: t, Title: title, EntityType: repository.EntityTypeOrganization, EntityID: orgID.String()}
}

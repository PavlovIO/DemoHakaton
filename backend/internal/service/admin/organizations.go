package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/kyc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/pgconv"
)

// OrganizationFilter — фильтры очереди компаний. nil и пустой срез — фильтр не задан.
type OrganizationFilter struct {
	KycStatuses []string
	Type        *string
	CountryCode *string
	Q           *string // подстрока названия или рег. номера
	Page        int
	PageSize    int
}

// OrganizationPage — страница компаний с бенефициарами.
type OrganizationPage struct {
	Items    []kyc.OrganizationProfile
	Total    int64
	Page     int
	PageSize int
}

// OrgShort — компания в AML-флаге.
type OrgShort struct {
	ID          uuid.UUID
	Name        string
	CountryCode string
}

// AmlFlag — AML-флаг в карточке компании.
type AmlFlag struct {
	ID                int64
	Organization      OrgShort
	ApplicationID     *uuid.UUID
	Rule              string
	Severity          string
	Details           string
	Status            string
	ResolvedByUserID  *uuid.UUID
	ResolutionComment *string
	CreatedAt         time.Time
	ResolvedAt        *time.Time
}

// OrganizationDetails — полная карточка компании для проверки KYC. Срезы не nil: в ответе
// [], а не null. RegistryCheck nil — сверки с реестром не было.
type OrganizationDetails struct {
	Profile       kyc.OrganizationProfile
	Documents     []repository.KycDocument
	AmlFlags      []AmlFlag
	Users         []User
	RegistryCheck *registry.Result
}

// ListOrganizations — очередь верификации и поиск компаний.
func (a *Admin) ListOrganizations(ctx context.Context, p service.Principal, f OrganizationFilter) (OrganizationPage, error) {
	if err := requireAdmin(p); err != nil {
		return OrganizationPage{}, err
	}
	page, size, limit, offset := service.PageWindow(f.Page, f.PageSize)
	count := repository.CountOrganizationsParams{
		KycStatuses: emptyToNil(f.KycStatuses),
		CountryCode: pgconv.Text(f.CountryCode),
		Q:           likeQuery(f.Q),
	}
	if f.Type != nil {
		count.Type = repository.NullOrganizationType{OrganizationType: repository.OrganizationType(*f.Type), Valid: true}
	}
	out := OrganizationPage{Page: page, PageSize: size}
	// Страница, total и бенефициары — в одном снимке: иначе компания, зарегистрированная
	// между запросами, попадёт в total, но не в items.
	err := a.tx.ReadSnapshot(ctx, func(q repository.Querier) error {
		orgs, err := q.ListOrganizations(ctx, repository.ListOrganizationsParams{
			Limit: limit, Offset: offset,
			KycStatuses: count.KycStatuses, Type: count.Type, CountryCode: count.CountryCode, Q: count.Q,
		})
		if err != nil {
			return fmt.Errorf("list organizations: %w", err)
		}
		if out.Total, err = q.CountOrganizations(ctx, count); err != nil {
			return fmt.Errorf("count organizations: %w", err)
		}
		out.Items, err = withBeneficiaries(ctx, q, orgs)
		return err
	})
	if err != nil {
		return OrganizationPage{}, err
	}
	return out, nil
}

// withBeneficiaries — бенефициары страницы одним запросом: Organization в контракте
// требует beneficiaries и в списке.
func withBeneficiaries(ctx context.Context, q repository.Querier, orgs []repository.Organization) ([]kyc.OrganizationProfile, error) {
	ids := make([]uuid.UUID, len(orgs))
	for i, o := range orgs {
		ids[i] = o.ID
	}
	rows, err := q.ListBeneficiariesByOrganizations(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list beneficiaries: %w", err)
	}
	byOrg := make(map[uuid.UUID][]kyc.Beneficiary, len(orgs))
	for _, b := range rows {
		byOrg[b.OrganizationID] = append(byOrg[b.OrganizationID], kyc.Beneficiary{
			ID: b.ID, FullName: b.FullName, CitizenshipCountryCode: b.CitizenshipCountryCode, OwnershipPercent: b.OwnershipPercent,
		})
	}
	items := make([]kyc.OrganizationProfile, len(orgs))
	for i, o := range orgs {
		list := byOrg[o.ID]
		if list == nil {
			list = []kyc.Beneficiary{}
		}
		items[i] = kyc.OrganizationProfile{Org: service.OrgFromRepository(o), Beneficiaries: list}
	}
	return items, nil
}

// GetOrganization — карточка компании: профиль, документы, AML-флаги, пользователи и
// последняя сверка с реестром.
func (a *Admin) GetOrganization(ctx context.Context, p service.Principal, orgID uuid.UUID) (OrganizationDetails, error) {
	if err := requireAdmin(p); err != nil {
		return OrganizationDetails{}, err
	}
	var out OrganizationDetails
	// Все части карточки — в одном снимке: админ решает по ней, и статус KYC из одного
	// состояния БД рядом с флагами из другого (решение другого админа между запросами)
	// ввёл бы его в заблуждение.
	err := a.tx.ReadSnapshot(ctx, func(q repository.Querier) error {
		var err error
		out, err = loadOrganizationDetails(ctx, q, orgID)
		return err
	})
	if err != nil {
		return OrganizationDetails{}, err
	}
	return out, nil
}

func loadOrganizationDetails(ctx context.Context, q repository.Querier, orgID uuid.UUID) (OrganizationDetails, error) {
	profile, err := kyc.LoadProfile(ctx, q, orgID)
	if err != nil {
		return OrganizationDetails{}, err
	}
	docs, err := q.ListKycDocumentsByOrg(ctx, orgID)
	if err != nil {
		return OrganizationDetails{}, fmt.Errorf("list kyc documents: %w", err)
	}
	flagRows, err := q.ListAmlFlagsByOrganization(ctx, orgID)
	if err != nil {
		return OrganizationDetails{}, fmt.Errorf("list aml flags: %w", err)
	}
	userRows, err := q.ListUsersByOrganization(ctx, pgtype.UUID{Bytes: orgID, Valid: true})
	if err != nil {
		return OrganizationDetails{}, fmt.Errorf("list organization users: %w", err)
	}
	check, err := latestRegistryCheck(ctx, q, orgID)
	if err != nil {
		return OrganizationDetails{}, err
	}

	out := OrganizationDetails{
		Profile:       profile,
		Documents:     docs,
		AmlFlags:      make([]AmlFlag, len(flagRows)),
		Users:         make([]User, len(userRows)),
		RegistryCheck: check,
	}
	if out.Documents == nil {
		out.Documents = []repository.KycDocument{}
	}
	for i, r := range flagRows {
		out.AmlFlags[i] = amlFlagFromParts(r.AmlFlag, r.OrganizationName, r.OrganizationCountryCode)
	}
	for i, u := range userRows {
		out.Users[i] = userFromListRow(repository.ListUsersRow(u))
	}
	return out, nil
}

// latestRegistryCheck — последний снапшот реестра; нет сверки — nil, а не 404.
func latestRegistryCheck(ctx context.Context, q repository.Querier, orgID uuid.UUID) (*registry.Result, error) {
	rc, err := q.GetLatestRegistryCheck(ctx, pgtype.UUID{Bytes: orgID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest registry check: %w", err)
	}
	return &registry.Result{
		Source:             registry.Source(rc.Source),
		Query:              rc.Query,
		Found:              rc.Found,
		Name:               rc.Name.String,
		RegistrationNumber: rc.RegistrationNumber.String,
		LegalAddress:       rc.LegalAddress.String,
		Status:             rc.RegistryStatus.String,
		CheckedAt:          rc.CheckedAt.Time,
	}, nil
}

// amlFlagFromParts — флаг и организация из строк ListAmlFlags*, GetAmlFlagByID: sqlc
// генерирует на каждый запрос свой тип строки.
func amlFlagFromParts(f repository.AmlFlag, orgName, orgCountryCode string) AmlFlag {
	return AmlFlag{
		ID:                f.ID,
		Organization:      OrgShort{ID: f.OrganizationID, Name: orgName, CountryCode: orgCountryCode},
		ApplicationID:     pgconv.UUIDPtr(f.ApplicationID),
		Rule:              string(f.Rule),
		Severity:          string(f.Severity),
		Details:           f.Details,
		Status:            string(f.Status),
		ResolvedByUserID:  pgconv.UUIDPtr(f.ResolvedByUserID),
		ResolutionComment: pgconv.TextPtr(f.ResolutionComment),
		CreatedAt:         f.CreatedAt.Time,
		ResolvedAt:        pgconv.TimePtr(f.ResolvedAt),
	}
}

// likeQuery — q для ILIKE … ESCAPE '\': пустой — фильтр не задан, иначе экранирован.
func likeQuery(q *string) pgtype.Text {
	if q == nil || *q == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: service.EscapeLike(*q), Valid: true}
}

// emptyToNil — пустой срез фильтра → NULL в запросе (фильтр не задан).
func emptyToNil(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

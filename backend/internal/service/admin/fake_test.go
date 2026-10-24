package admin_test

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeDB — БД админки в памяти: одна организация, её флаги, заявки и пользователи.
// Транзакция (fakeTx) работает с ней же: откат не моделируется, тесты проверяют, какие
// записи дошли до querier, а не до коммита.
type fakeDB struct {
	repository.Querier
	org           *repository.Organization
	openHighFlags int64
	applications  map[uuid.UUID]repository.Application
	users         map[uuid.UUID]repository.User
	beneficiaries []repository.Beneficiary
	documents     []repository.KycDocument
	flags         []repository.ListAmlFlagsByOrganizationRow
	orgUsers      []repository.ListUsersByOrganizationRow
	registryCheck *repository.RegistryCheck
	listOrgs      []repository.Organization
	listUsers     []repository.ListUsersRow
	total         int64

	// raceStatus — UPDATE с охраной статуса не нашёл строку: «параллельное» решение.
	raceStatus bool

	approvedKyc     bool
	rejectedKyc     *repository.RejectKycParams
	approvedApp     *repository.ApproveApplicationParams
	rejectedApp     *repository.RejectApplicationParams
	blocked         *repository.BlockUserParams
	unblocked       bool
	deletedSessions []uuid.UUID
	history         []repository.CreateStatusHistoryParams
	orgNotices      []repository.CreateNotificationsForOrganizationParams
	audit           []repository.CreateAuditLogParams
	orgListArg      *repository.ListOrganizationsParams
	orgCountArg     *repository.CountOrganizationsParams
	userListArg     *repository.ListUsersParams
	userCountArg    *repository.CountUsersParams
	amlFlags        map[int64]repository.AmlFlag
	flagListArg     *repository.ListAmlFlagsParams
	flagCountArg    *repository.CountAmlFlagsParams
	snapshots       int // вызовы ReadSnapshot: чтение из нескольких запросов должно идти в одном
}

func newFakeDB() *fakeDB {
	return &fakeDB{applications: map[uuid.UUID]repository.Application{}, users: map[uuid.UUID]repository.User{}}
}

func (f *fakeDB) withOrg(status repository.KycStatus) *repository.Organization {
	f.org = &repository.Organization{
		ID: uuid.New(), Type: repository.OrganizationTypeBorrower, Name: "ООО Альфа", CountryCode: "RU",
		RegistrationNumber: "7707083893", LegalAddress: "Москва", KycStatus: status,
	}
	return f.org
}

type fakeTx struct{ db *fakeDB }

func (t fakeTx) InTx(_ context.Context, fn func(q repository.Querier) error) error { return fn(t.db) }

func (t fakeTx) ReadSnapshot(_ context.Context, fn func(q repository.Querier) error) error {
	t.db.snapshots++
	return fn(t.db)
}

func adminPrincipal() service.Principal {
	return service.Principal{UserID: uuid.New(), Role: string(repository.UserRoleAdmin)}
}

func borrowerPrincipal() service.Principal {
	orgID := uuid.New()
	return service.Principal{UserID: uuid.New(), OrgID: &orgID, Role: string(repository.UserRoleBorrower), KycStatus: "verified"}
}

func (f *fakeDB) GetOrganizationByID(_ context.Context, id uuid.UUID) (repository.Organization, error) {
	if f.org == nil || f.org.ID != id {
		return repository.Organization{}, pgx.ErrNoRows
	}
	return *f.org, nil
}

func (f *fakeDB) GetOrganizationForUpdate(ctx context.Context, id uuid.UUID) (repository.Organization, error) {
	return f.GetOrganizationByID(ctx, id)
}

func (f *fakeDB) ListBeneficiariesByOrg(ctx context.Context, orgID uuid.UUID) ([]repository.Beneficiary, error) {
	return f.ListBeneficiariesByOrganizations(ctx, []uuid.UUID{orgID})
}

func (f *fakeDB) ListBeneficiariesByOrganizations(_ context.Context, ids []uuid.UUID) ([]repository.Beneficiary, error) {
	var out []repository.Beneficiary
	for _, b := range f.beneficiaries {
		if slices.Contains(ids, b.OrganizationID) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeDB) ListKycDocumentsByOrg(context.Context, uuid.UUID) ([]repository.KycDocument, error) {
	return f.documents, nil
}

func (f *fakeDB) ListAmlFlagsByOrganization(context.Context, uuid.UUID) ([]repository.ListAmlFlagsByOrganizationRow, error) {
	return f.flags, nil
}

func (f *fakeDB) ListUsersByOrganization(context.Context, pgtype.UUID) ([]repository.ListUsersByOrganizationRow, error) {
	return f.orgUsers, nil
}

func (f *fakeDB) GetLatestRegistryCheck(context.Context, pgtype.UUID) (repository.RegistryCheck, error) {
	if f.registryCheck == nil {
		return repository.RegistryCheck{}, pgx.ErrNoRows
	}
	return *f.registryCheck, nil
}

func (f *fakeDB) ListOrganizations(_ context.Context, arg repository.ListOrganizationsParams) ([]repository.Organization, error) {
	f.orgListArg = &arg
	return f.listOrgs, nil
}

func (f *fakeDB) CountOrganizations(_ context.Context, arg repository.CountOrganizationsParams) (int64, error) {
	f.orgCountArg = &arg
	return f.total, nil
}

func (f *fakeDB) ListUsers(_ context.Context, arg repository.ListUsersParams) ([]repository.ListUsersRow, error) {
	f.userListArg = &arg
	return f.listUsers, nil
}

func (f *fakeDB) CountUsers(_ context.Context, arg repository.CountUsersParams) (int64, error) {
	f.userCountArg = &arg
	return f.total, nil
}

func (f *fakeDB) CountOpenHighAmlFlags(context.Context, uuid.UUID) (int64, error) {
	return f.openHighFlags, nil
}

func (f *fakeDB) ApproveKyc(_ context.Context, id uuid.UUID) (repository.Organization, error) {
	if f.raceStatus || f.org.KycStatus != repository.KycStatusUnderReview {
		return repository.Organization{}, pgx.ErrNoRows
	}
	f.approvedKyc = true
	f.org.KycStatus = repository.KycStatusVerified
	f.org.VerifiedAt = pgtype.Timestamptz{Valid: true}
	return *f.org, nil
}

func (f *fakeDB) RejectKyc(_ context.Context, arg repository.RejectKycParams) (repository.Organization, error) {
	if f.raceStatus || f.org.KycStatus != repository.KycStatusUnderReview {
		return repository.Organization{}, pgx.ErrNoRows
	}
	f.rejectedKyc = &arg
	f.org.KycStatus = repository.KycStatusRejected
	f.org.KycRejectReason = arg.KycRejectReason
	return *f.org, nil
}

func (f *fakeDB) CreateStatusHistory(_ context.Context, arg repository.CreateStatusHistoryParams) (repository.StatusHistory, error) {
	f.history = append(f.history, arg)
	return repository.StatusHistory{}, nil
}

func (f *fakeDB) CreateNotificationsForOrganization(_ context.Context, arg repository.CreateNotificationsForOrganizationParams) (int64, error) {
	f.orgNotices = append(f.orgNotices, arg)
	return 1, nil
}

func (f *fakeDB) CreateAuditLog(_ context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error) {
	f.audit = append(f.audit, arg)
	return repository.AuditLog{}, nil
}

func (f *fakeDB) withApplication(status repository.ApplicationStatus) repository.Application {
	app := repository.Application{
		ID: uuid.New(), Number: pgtype.Int8{Int64: 42, Valid: true}, BorrowerOrganizationID: uuid.New(),
		Status: status, Currency: "RUB", TargetCountryCode: "RU",
	}
	f.applications[app.ID] = app
	return app
}

func (f *fakeDB) LockApplicationByID(_ context.Context, id uuid.UUID) (repository.Application, error) {
	app, ok := f.applications[id]
	if !ok {
		return repository.Application{}, pgx.ErrNoRows
	}
	return app, nil
}

// transitionApplication — UPDATE … AND status = from_status.
func (f *fakeDB) transitionApplication(id uuid.UUID, from, to repository.ApplicationStatus, change func(*repository.Application)) (repository.Application, error) {
	app, ok := f.applications[id]
	if !ok || f.raceStatus || app.Status != from {
		return repository.Application{}, pgx.ErrNoRows
	}
	app.Status = to
	change(&app)
	f.applications[id] = app
	return app, nil
}

func (f *fakeDB) ApproveApplication(_ context.Context, arg repository.ApproveApplicationParams) (repository.Application, error) {
	f.approvedApp = &arg
	return f.transitionApplication(arg.ID, arg.FromStatus, repository.ApplicationStatusPublished, func(a *repository.Application) {
		a.PublishedAt = pgtype.Timestamptz{Valid: true}
	})
}

func (f *fakeDB) RejectApplication(_ context.Context, arg repository.RejectApplicationParams) (repository.Application, error) {
	f.rejectedApp = &arg
	return f.transitionApplication(arg.ID, arg.FromStatus, repository.ApplicationStatusRejected, func(a *repository.Application) {
		a.ModerationComment = arg.ModerationComment
	})
}

func (f *fakeDB) GetApplicationVisible(_ context.Context, arg repository.GetApplicationVisibleParams) (repository.GetApplicationVisibleRow, error) {
	app, ok := f.applications[arg.ID]
	if !ok || arg.ViewerRole != repository.UserRoleAdmin {
		return repository.GetApplicationVisibleRow{}, pgx.ErrNoRows
	}
	return repository.GetApplicationVisibleRow{Application: app, BorrowerName: "ООО Альфа", BorrowerCountryCode: "RU"}, nil
}

func (f *fakeDB) withUser(role repository.UserRole, status repository.UserStatus) repository.User {
	u := repository.User{ID: uuid.New(), Email: string(role) + "@example.com", Role: role, Status: status}
	if role != repository.UserRoleAdmin {
		u.OrganizationID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
	}
	f.users[u.ID] = u
	return u
}

func (f *fakeDB) GetUserByID(_ context.Context, id uuid.UUID) (repository.User, error) {
	u, ok := f.users[id]
	if !ok {
		return repository.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func (f *fakeDB) GetUserForUpdate(ctx context.Context, id uuid.UUID) (repository.User, error) {
	return f.GetUserByID(ctx, id)
}

func (f *fakeDB) BlockUser(_ context.Context, arg repository.BlockUserParams) (repository.BlockUserRow, error) {
	f.blocked = &arg
	u := f.users[arg.ID]
	u.Status, u.BlockedReason = repository.UserStatusBlocked, arg.BlockedReason
	f.users[arg.ID] = u
	return repository.BlockUserRow{ID: u.ID, Email: u.Email, Role: u.Role, Status: u.Status, OrganizationID: u.OrganizationID}, nil
}

func (f *fakeDB) UnblockUser(_ context.Context, id uuid.UUID) (repository.UnblockUserRow, error) {
	f.unblocked = true
	u := f.users[id]
	u.Status, u.BlockedReason = repository.UserStatusActive, pgtype.Text{}
	f.users[id] = u
	return repository.UnblockUserRow{ID: u.ID, Email: u.Email, Role: u.Role, Status: u.Status, OrganizationID: u.OrganizationID}, nil
}

func (f *fakeDB) DeleteSessionsByUserID(_ context.Context, userID uuid.UUID) error {
	f.deletedSessions = append(f.deletedSessions, userID)
	return nil
}

// withAmlFlag добавляет флаг организации org в amlFlags.
func (f *fakeDB) withAmlFlag(org *repository.Organization, status repository.AmlFlagStatus) repository.AmlFlag {
	if f.amlFlags == nil {
		f.amlFlags = map[int64]repository.AmlFlag{}
	}
	flag := repository.AmlFlag{
		ID: int64(len(f.amlFlags) + 1), OrganizationID: org.ID, Rule: repository.AmlRuleSanctionsMatch,
		Severity: repository.AmlSeverityHigh, Details: "Совпадение", Status: status,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.amlFlags[flag.ID] = flag
	return flag
}

func (f *fakeDB) ResolveAmlFlag(_ context.Context, arg repository.ResolveAmlFlagParams) (repository.AmlFlag, error) {
	flag, ok := f.amlFlags[arg.ID]
	if !ok || flag.Status != repository.AmlFlagStatusOpen {
		return repository.AmlFlag{}, pgx.ErrNoRows
	}
	flag.Status, flag.ResolvedByUserID, flag.ResolutionComment = arg.Status, arg.ResolvedByUserID, arg.ResolutionComment
	flag.ResolvedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	f.amlFlags[arg.ID] = flag
	return flag, nil
}

func (f *fakeDB) GetAmlFlagByID(_ context.Context, id int64) (repository.GetAmlFlagByIDRow, error) {
	flag, ok := f.amlFlags[id]
	if !ok {
		return repository.GetAmlFlagByIDRow{}, pgx.ErrNoRows
	}
	return repository.GetAmlFlagByIDRow{AmlFlag: flag, OrganizationName: f.org.Name, OrganizationCountryCode: f.org.CountryCode}, nil
}

func (f *fakeDB) ListAmlFlags(_ context.Context, arg repository.ListAmlFlagsParams) ([]repository.ListAmlFlagsRow, error) {
	f.flagListArg = &arg
	var rows []repository.ListAmlFlagsRow
	for id := int64(1); id <= int64(len(f.amlFlags)); id++ {
		rows = append(rows, repository.ListAmlFlagsRow{AmlFlag: f.amlFlags[id], OrganizationName: f.org.Name, OrganizationCountryCode: f.org.CountryCode})
	}
	return rows, nil
}

func (f *fakeDB) CountAmlFlags(_ context.Context, arg repository.CountAmlFlagsParams) (int64, error) {
	f.flagCountArg = &arg
	return f.total, nil
}

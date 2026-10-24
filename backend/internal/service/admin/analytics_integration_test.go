package admin_test

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/admin"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// Сводка на заселённой БД: фикстуры датированы 2001 годом, демо-сид в период не попадает.
// Строки 2003 года проверяют, что период работает для каждого поля.
func TestAnalyticsSummaryIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	adm := insertAdmin(ctx, t, tx)
	borrower := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleBorrower, "Analytics B")
	lenderCY := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleLender, "Analytics CY")
	lenderAE := insertLender(ctx, t, tx, "AE")

	in2001 := time.Date(2001, 4, 1, 0, 0, 0, 0, time.UTC)
	in2003 := time.Date(2003, 4, 1, 0, 0, 0, 0, time.UTC)

	application := func(status, amount, currency, country string, at time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO applications (borrower_organization_id, created_by_user_id, status,
				amount, currency, target_country_code, purpose, term_months, transfer_method, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'пополнение оборотных средств', 12, 'swift', $7) RETURNING id`,
			*borrower.OrgID, borrower.UserID, status, amount, currency, country, at).Scan(&id)
		if err != nil {
			t.Fatalf("application %s %s: %v", status, amount, err)
		}
		return id
	}
	offer := func(applicationID uuid.UUID, lender uuid.UUID, lenderUser uuid.UUID, at time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO offers (application_id, lender_organization_id, created_by_user_id,
				amount, currency, interest_rate_annual, repayment_schedule, created_at)
			SELECT id, $2, $3, amount, currency, 12, 'bullet', $4 FROM applications WHERE id = $1 RETURNING id`,
			applicationID, lender, lenderUser, at).Scan(&id)
		if err != nil {
			t.Fatalf("offer: %v", err)
		}
		return id
	}
	deal := func(applicationID, offerID uuid.UUID, at time.Time) {
		t.Helper()
		_, err := tx.Exec(ctx, `INSERT INTO deals (application_id, offer_id, borrower_organization_id, lender_organization_id,
				amount, currency, interest_rate_annual, term_months, target_country_code, transfer_method,
				repayment_schedule, agreed_at)
			SELECT a.id, o.id, a.borrower_organization_id, o.lender_organization_id, o.amount, o.currency,
			       o.interest_rate_annual, a.term_months, a.target_country_code, a.transfer_method,
			       o.repayment_schedule, $3
			FROM applications a JOIN offers o ON o.id = $2 WHERE a.id = $1`, applicationID, offerID, at)
		if err != nil {
			t.Fatalf("deal: %v", err)
		}
	}
	agree := func(applicationID, offerID uuid.UUID) {
		t.Helper()
		if _, err := tx.Exec(ctx, `UPDATE offers SET status = 'accepted', decided_at = created_at WHERE id = $1`, offerID); err != nil {
			t.Fatalf("accept offer: %v", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE applications SET status = 'agreed', accepted_offer_id = $2 WHERE id = $1`, applicationID, offerID); err != nil {
			t.Fatalf("agree application: %v", err)
		}
	}

	// 2001: заявки в 4 статусах и 2 валютах, 3 оффера от кредиторов из 2 стран, 1 сделка.
	publishedEUR := application("published", "100.00", "EUR", "DE", in2001)
	agreedEUR := application("published", "200.50", "EUR", "DE", in2001)
	publishedUSD := application("published", "300.00", "USD", "CN", in2001)
	application("draft", "999.00", "USD", "US", in2001)
	application("moderation", "50.00", "USD", "US", in2001)
	accepted := offer(agreedEUR, *lenderCY.OrgID, lenderCY.UserID, in2001)
	agree(agreedEUR, accepted)
	offer(publishedEUR, *lenderCY.OrgID, lenderCY.UserID, in2001)
	offer(publishedUSD, lenderAE.orgID, lenderAE.userID, in2001)
	deal(agreedEUR, accepted, in2001)

	// 2003: по строке каждого вида вне периода.
	late := application("published", "1000.00", "EUR", "DE", in2003)
	lateOffer := offer(late, lenderAE.orgID, lenderAE.userID, in2003)
	deal(late, lateOffer, in2003)

	from := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newIntegrationAdmin(tx)

	got, err := svc.AnalyticsSummary(ctx, adm, admin.AnalyticsFilter{CreatedFrom: &from, CreatedTo: &to, TopN: 10})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := admin.AnalyticsSummary{
		ApplicationsTotal:    5,
		ApplicationsByStatus: map[string]int64{"published": 2, "agreed": 1, "draft": 1, "moderation": 1},
		OffersTotal:          3,
		DealsTotal:           1,
		// draft не входит в суммы: спрос, который не подан.
		AmountsByCurrency: []admin.CurrencyStat{
			{Currency: "EUR", Count: 2, TotalAmount: decimal.RequireFromString("300.50"), AverageAmount: decimal.RequireFromString("150.25")},
			{Currency: "USD", Count: 2, TotalAmount: decimal.RequireFromString("350.00"), AverageAmount: decimal.RequireFromString("175.00")},
		},
		// Спрос — только published/agreed: moderation в US не считается.
		TopDemandCountries: []admin.CountryStat{{CountryCode: "DE", Count: 2}, {CountryCode: "CN", Count: 1}},
		TopSupplyCountries: []admin.CountryStat{{CountryCode: "CY", Count: 2}, {CountryCode: "AE", Count: 1}},
	}
	if got.ApplicationsTotal != want.ApplicationsTotal || got.OffersTotal != want.OffersTotal || got.DealsTotal != want.DealsTotal {
		t.Errorf("totals = %d/%d/%d, want %d/%d/%d", got.ApplicationsTotal, got.OffersTotal, got.DealsTotal,
			want.ApplicationsTotal, want.OffersTotal, want.DealsTotal)
	}
	if !maps.Equal(got.ApplicationsByStatus, want.ApplicationsByStatus) {
		t.Errorf("by status = %v, want %v", got.ApplicationsByStatus, want.ApplicationsByStatus)
	}
	if !slices.EqualFunc(got.AmountsByCurrency, want.AmountsByCurrency, func(a, b admin.CurrencyStat) bool {
		return a.Currency == b.Currency && a.Count == b.Count && a.TotalAmount.Equal(b.TotalAmount) && a.AverageAmount.Equal(b.AverageAmount)
	}) {
		t.Errorf("amounts = %v, want %v", got.AmountsByCurrency, want.AmountsByCurrency)
	}
	if !slices.Equal(got.TopDemandCountries, want.TopDemandCountries) {
		t.Errorf("top demand = %v, want %v", got.TopDemandCountries, want.TopDemandCountries)
	}
	if !slices.Equal(got.TopSupplyCountries, want.TopSupplyCountries) {
		t.Errorf("top supply = %v, want %v", got.TopSupplyCountries, want.TopSupplyCountries)
	}

	t.Run("top_n обрезает топы", func(t *testing.T) {
		got, err := svc.AnalyticsSummary(ctx, adm, admin.AnalyticsFilter{CreatedFrom: &from, CreatedTo: &to, TopN: 1})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got.TopDemandCountries) != 1 || got.TopDemandCountries[0].CountryCode != "DE" ||
			len(got.TopSupplyCountries) != 1 || got.TopSupplyCountries[0].CountryCode != "CY" {
			t.Fatalf("tops = %+v / %+v, want only DE / CY", got.TopDemandCountries, got.TopSupplyCountries)
		}
	})
}

type lenderFixture struct{ orgID, userID uuid.UUID }

// insertLender — кредитор из country: InsertVerifiedOrg всегда ставит CY.
func insertLender(ctx context.Context, t *testing.T, db servicetest.RowQuerier, country string) lenderFixture {
	t.Helper()
	var orgID uuid.UUID
	err := db.QueryRow(ctx, `INSERT INTO organizations (type, name, country_code, registration_number, legal_address,
			kyc_status, verified_at)
		VALUES ('lender', 'Analytics '||$1, $1, $2, 'Адрес', 'verified', now()) RETURNING id`,
		country, servicetest.UniqueRegistrationNumber()).Scan(&orgID)
	if err != nil {
		t.Fatalf("lender %s: %v", country, err)
	}
	return lenderFixture{orgID: orgID, userID: servicetest.InsertUser(ctx, t, db, repository.UserRoleLender, &orgID)}
}

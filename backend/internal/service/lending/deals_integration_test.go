package lending_test

// Интеграционные тесты принятия оффера и сделок на настоящем Postgres.
//   - TestDealsIntegration — сценарий issue, повтор, чужой оффер, просрочка, видимость;
//     всё в транзакции с откатом (сервис открывает savepoint через tx.Begin).
//   - TestAcceptOfferConcurrent — параллельные accept: настоящие транзакции на пуле с
//     коммитами, иначе FOR UPDATE нечего сериализовать. У каждой итерации свои заявка и
//     офферы; в конце servicetest.CleanupCommittedOrgs удаляет всё созданное.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// insertDealsApp — опубликованная заявка 500 000.00 EUR, DE, 12 мес., sepa.
func insertDealsApp(ctx context.Context, t *testing.T, db servicetest.RowQuerier, b service.Principal) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.QueryRow(ctx, `INSERT INTO applications (borrower_organization_id, created_by_user_id, status,
			amount, currency, target_country_code, purpose, term_months, transfer_method)
		VALUES ($1, $2, 'published', 500000.00, 'EUR', 'DE', 'Закупка оборудования', 12, 'sepa')
		RETURNING id`, b.ViewerOrg(), b.UserID).Scan(&id)
	if err != nil {
		t.Fatalf("application: %v", err)
	}
	return id
}

// insertDealsOffer — sent-оффер 500 000.00 EUR, 8 %, комиссия 1 %. SQL, а не Offers.Create:
// нужен и просроченный valid_until, который сервис не создаёт.
func insertDealsOffer(ctx context.Context, t *testing.T, db servicetest.RowQuerier, appID uuid.UUID, l service.Principal, validUntil *time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.QueryRow(ctx, `INSERT INTO offers (application_id, lender_organization_id, created_by_user_id,
			amount, currency, interest_rate_annual, fee_percent, repayment_schedule, valid_until)
		VALUES ($1, $2, $3, 500000.00, 'EUR', 8, 1, 'annuity', $4) RETURNING id`,
		appID, l.ViewerOrg(), l.UserID, validUntil).Scan(&id)
	if err != nil {
		t.Fatalf("offer: %v", err)
	}
	return id
}

func TestDealsIntegration(t *testing.T) {
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	q := repository.New(tx)
	deals := lending.NewDeals(lending.DealsDeps{Store: q, Tx: service.NewTxManager(tx)})
	count := func(t *testing.T, sql string, args ...any) int {
		t.Helper()
		var n int
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}

	b1 := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleBorrower, "B1")
	b2 := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleBorrower, "B2")
	l1 := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleLender, "L1")
	l2 := servicetest.InsertVerifiedOrg(ctx, t, tx, repository.UserRoleLender, "L2")
	admin := service.Principal{UserID: servicetest.InsertUser(ctx, t, tx, repository.UserRoleAdmin, nil), Role: "admin"}
	appID := insertDealsApp(ctx, t, tx, b1)
	o1 := insertDealsOffer(ctx, t, tx, appID, l1, nil)
	o2 := insertDealsOffer(ctx, t, tx, appID, l2, nil)

	deal, err := deals.AcceptOffer(ctx, b1, service.ClientMeta{}, o1)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	t.Run("сценарий issue: 500 000.00 EUR × 1 % = 5 000.00", func(t *testing.T) {
		if deal.Amount.StringFixed(2) != "500000.00" || deal.Currency != "EUR" ||
			deal.FeeAmount == nil || deal.FeeAmount.StringFixed(2) != "5000.00" ||
			deal.Borrower.Name != "Deals B1" || deal.Lender.Name != "Deals L1" ||
			deal.TermMonths != 12 || deal.TargetCountryCode != "DE" || deal.TransferMethod != "sepa" {
			t.Errorf("deal = %+v", deal)
		}
		var status string
		var accepted pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT status, accepted_offer_id FROM applications WHERE id = $1`, appID).
			Scan(&status, &accepted); err != nil || status != "agreed" || accepted != (pgtype.UUID{Bytes: o1, Valid: true}) {
			t.Errorf("заявка: %s %v, %v", status, accepted, err)
		}
		if n := count(t, `SELECT count(*) FROM offers WHERE id = $1 AND status = 'rejected'
			AND reject_reason = 'another_offer_accepted'`, o2); n != 1 {
			t.Errorf("второй оффер не отклонён")
		}
		if n := count(t, `SELECT count(*) FROM status_history WHERE entity_id = ANY($1) AND actor_user_id = $2`,
			[]uuid.UUID{appID, o1, o2}, b1.UserID); n != 3 {
			t.Errorf("status_history = %d, want 3", n)
		}
		if n := count(t, `SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'offer_accepted'
			AND entity_type = 'deal' AND entity_id = $2`, l1.UserID, deal.ID.String()); n != 1 {
			t.Errorf("offer_accepted кредитору L1: %d", n)
		}
		if n := count(t, `SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'offer_rejected'
			AND entity_id = $2`, l2.UserID, o2.String()); n != 1 {
			t.Errorf("offer_rejected кредитору L2: %d", n)
		}
		if n := count(t, `SELECT count(*) FROM notifications WHERE user_id = $1`, b1.UserID); n != 0 {
			t.Errorf("заёмщику уведомлений: %d, want 0", n)
		}
		if n := count(t, `SELECT count(*) FROM audit_log WHERE action = 'offer.accept' AND entity_id = $1
			AND meta->>'deal_id' = $2`, o1.String(), deal.ID.String()); n != 1 {
			t.Errorf("audit offer.accept: %d", n)
		}
	})

	t.Run("повторный accept — INVALID_STATUS_TRANSITION, сделка одна", func(t *testing.T) {
		_, err := deals.AcceptOffer(ctx, b1, service.ClientMeta{}, o1)
		assertCode(t, "repeat", err, apperr.CodeInvalidTransition)
		if n := count(t, `SELECT count(*) FROM deals WHERE application_id = $1`, appID); n != 1 {
			t.Errorf("deals = %d, want 1", n)
		}
	})

	t.Run("чужой и несуществующий оффер — NOT_FOUND", func(t *testing.T) {
		other := insertDealsOffer(ctx, t, tx, insertDealsApp(ctx, t, tx, b1), l1, nil)
		_, err := deals.AcceptOffer(ctx, b2, service.ClientMeta{}, other)
		assertCode(t, "чужой", err, apperr.CodeNotFound)
		_, err = deals.AcceptOffer(ctx, b1, service.ClientMeta{}, uuid.New())
		assertCode(t, "несуществующий", err, apperr.CodeNotFound)
	})

	t.Run("просроченный оффер — INVALID_STATUS_TRANSITION, оффер остаётся sent", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		expired := insertDealsOffer(ctx, t, tx, insertDealsApp(ctx, t, tx, b1), l1, &past)
		_, err := deals.AcceptOffer(ctx, b1, service.ClientMeta{}, expired)
		assertCode(t, "expired", err, apperr.CodeInvalidTransition)
		if n := count(t, `SELECT count(*) FROM offers WHERE id = $1 AND status = 'sent'`, expired); n != 1 {
			t.Errorf("статус просроченного оффера изменился")
		}
	})

	t.Run("видимость сделок", func(t *testing.T) {
		for name, p := range map[string]service.Principal{"заёмщик": b1, "кредитор": l1, "admin": admin} {
			if got, err := deals.Get(ctx, p, deal.ID); err != nil || got.ID != deal.ID {
				t.Errorf("%s getDeal: %+v, %v", name, got, err)
			}
		}
		_, err := deals.Get(ctx, l2, deal.ID)
		assertCode(t, "другой кредитор getDeal", err, apperr.CodeNotFound)

		total := func(p service.Principal, f lending.DealFilter) int64 {
			t.Helper()
			page, err := deals.List(ctx, p, f)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			return page.Total
		}
		if n := total(l2, lending.DealFilter{}); n != 0 {
			t.Errorf("другой кредитор listDeals total = %d", n)
		}
		nilLender := service.Principal{UserID: l2.UserID, Role: "lender"} // OrgID nil → viewer_org = uuid.Nil
		if n := total(nilLender, lending.DealFilter{}); n != 0 {
			t.Errorf("viewer_org = uuid.Nil total = %d", n)
		}
		l1Org := l1.ViewerOrg()
		if page, err := deals.List(ctx, admin, lending.DealFilter{LenderOrgID: &l1Org}); err != nil ||
			page.Total != 1 || page.Items[0].ID != deal.ID {
			t.Errorf("admin по кредитору L1: %+v, %v", page, err)
		}
		if n := total(b1, lending.DealFilter{}); n != 1 {
			t.Errorf("заёмщик listDeals total = %d", n)
		}
		later := time.Now().Add(time.Hour)
		if n := total(b1, lending.DealFilter{CreatedFrom: &later}); n != 0 {
			t.Errorf("created_from после agreed_at total = %d", n)
		}
	})
}

func TestAcceptOfferConcurrent(t *testing.T) {
	ctx := context.Background()
	pool := servicetest.NewPool(t)
	deals := lending.NewDeals(lending.DealsDeps{Store: repository.New(pool), Tx: service.NewTxManager(pool)})
	track := servicetest.CleanupCommittedOrgs(t)
	borrower := servicetest.InsertVerifiedOrg(ctx, t, pool, repository.UserRoleBorrower, "Concurrent B")
	track(*borrower.OrgID)
	lenderA := servicetest.InsertVerifiedOrg(ctx, t, pool, repository.UserRoleLender, "Concurrent LA")
	track(*lenderA.OrgID)
	lenderB := servicetest.InsertVerifiedOrg(ctx, t, pool, repository.UserRoleLender, "Concurrent LB")
	track(*lenderB.OrgID)

	// raceAccept — accept каждого id в своей горутине, старт одновременно.
	raceAccept := func(ids ...uuid.UUID) []error {
		start := make(chan struct{})
		errs := make([]error, len(ids))
		var wg sync.WaitGroup
		for i, id := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[i] = deals.AcceptOffer(ctx, borrower, service.ClientMeta{}, id)
			}()
		}
		close(start)
		wg.Wait()
		return errs
	}
	// winnerOf — ровно один nil, остальные INVALID_STATUS_TRANSITION; индекс победителя.
	winnerOf := func(t *testing.T, errs []error) int {
		t.Helper()
		winner := -1
		for i, err := range errs {
			switch {
			case err == nil && winner < 0:
				winner = i
			case apperr.CodeOf(err) != apperr.CodeInvalidTransition:
				t.Fatalf("errs = %v: want один nil и INVALID_STATUS_TRANSITION", errs)
			}
		}
		if winner < 0 {
			t.Fatalf("errs = %v: нет победителя", errs)
		}
		return winner
	}
	checkDeal := func(t *testing.T, pool *pgxpool.Pool, appID, accepted uuid.UUID) {
		t.Helper()
		var deals int
		var acceptedID pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM deals WHERE application_id = a.id), a.accepted_offer_id
			FROM applications a WHERE a.id = $1 AND a.status = 'agreed'`, appID).Scan(&deals, &acceptedID); err != nil {
			t.Fatalf("заявка не agreed: %v", err)
		}
		if deals != 1 || acceptedID != (pgtype.UUID{Bytes: accepted, Valid: true}) {
			t.Fatalf("deals = %d, accepted_offer_id = %v, want 1 и %s", deals, acceptedID, accepted)
		}
	}

	t.Run("два разных оффера одной заявки", func(t *testing.T) {
		for range 20 { // повтор повышает шанс настоящего пересечения транзакций
			appID := insertDealsApp(ctx, t, pool, borrower)
			offers := []uuid.UUID{
				insertDealsOffer(ctx, t, pool, appID, lenderA, nil),
				insertDealsOffer(ctx, t, pool, appID, lenderB, nil),
			}
			w := winnerOf(t, raceAccept(offers...))
			checkDeal(t, pool, appID, offers[w])
			var reason string
			if err := pool.QueryRow(ctx, `SELECT reject_reason FROM offers WHERE id = $1 AND status = 'rejected'`,
				offers[1-w]).Scan(&reason); err != nil || reason != "another_offer_accepted" {
				t.Fatalf("проигравший оффер: %q, %v", reason, err)
			}
		}
	})

	t.Run("один оффер дважды", func(t *testing.T) {
		for range 20 {
			appID := insertDealsApp(ctx, t, pool, borrower)
			offerID := insertDealsOffer(ctx, t, pool, appID, lenderA, nil)
			winnerOf(t, raceAccept(offerID, offerID))
			checkDeal(t, pool, appID, offerID)
		}
	})
}

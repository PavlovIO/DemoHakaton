package lending_test

// Интеграционный тест сервиса офферов на настоящем Postgres: видимость с JOIN заявки,
// unread_messages по message_reads, LockApplicationForLender и владелец в RejectOffer.
// Сервис открывает транзакции через tx.Begin (savepoint) — всё откатывается в Cleanup.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// offersFixture — компании и заявки плана BE-08:
//   - заёмщики B1, B2, кредиторы L1, L2, у каждой компании по пользователю;
//   - A1 (B1, published) — на неё создаются офферы; A2 (B2, draft) — чужой черновик;
//   - A3 (B1, cancelled) с оффером L1 — кредитору заявка видна, но офферы не принимает.
type offersFixture struct {
	tx     pgx.Tx
	q      *repository.Queries
	offers *lending.Offers
	p      map[string]service.Principal
	app    map[string]uuid.UUID
}

func newOffersFixture(t *testing.T) *offersFixture {
	t.Helper()
	ctx := context.Background()
	tx := servicetest.BeginTx(t)
	f := &offersFixture{
		tx: tx, q: repository.New(tx),
		p:   map[string]service.Principal{},
		app: map[string]uuid.UUID{},
	}
	f.offers = lending.NewOffers(lending.OffersDeps{Store: f.q, Tx: service.NewTxManager(tx)})

	orgs := []struct {
		name    string
		role    repository.UserRole
		country string
	}{
		{"B1", repository.UserRoleBorrower, "RU"}, {"B2", repository.UserRoleBorrower, "RU"},
		{"L1", repository.UserRoleLender, "AE"}, {"L2", repository.UserRoleLender, "CN"},
	}
	for _, o := range orgs {
		var orgID uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO organizations (type, name, country_code, registration_number, legal_address,
				kyc_status, verified_at)
			VALUES ($1, $2, $3, $4, 'Адрес', 'verified', now()) RETURNING id`,
			o.role, "Offers "+o.name, o.country, servicetest.UniqueRegistrationNumber()).Scan(&orgID)
		if err != nil {
			t.Fatalf("organization %s: %v", o.name, err)
		}
		userID := servicetest.InsertUser(ctx, t, tx, o.role, &orgID)
		f.p[o.name] = service.Principal{UserID: userID, OrgID: &orgID, Role: string(o.role), KycStatus: "verified"}
	}
	f.p["admin"] = service.Principal{UserID: servicetest.InsertUser(ctx, t, tx, repository.UserRoleAdmin, nil), Role: "admin"}

	apps := []struct{ name, borrower, status string }{
		{"A1", "B1", "published"}, {"A2", "B2", "draft"}, {"A3", "B1", "published"},
	}
	for _, a := range apps {
		b := f.p[a.borrower]
		var id uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO applications (borrower_organization_id, created_by_user_id, status,
				amount, currency, target_country_code, purpose, term_months, transfer_method)
			VALUES ($1, $2, $3, 1000000, 'RUB', 'AE', 'Пополнение оборотных средств', 12, 'swift')
			RETURNING id`, b.ViewerOrg(), b.UserID, a.status).Scan(&id)
		if err != nil {
			t.Fatalf("application %s: %v", a.name, err)
		}
		f.app[a.name] = id
	}
	// A3: оффер L1 появляется, пока заявка published, затем заёмщик её отменяет — как в
	// cancelApplication (BE-07): офферы закрываются с application_cancelled.
	o := f.create(t, "L1", "A3")
	if _, err := tx.Exec(ctx, `UPDATE offers SET status = 'rejected', reject_reason = 'application_cancelled',
			decided_at = now() WHERE id = $1`, o.ID); err != nil {
		t.Fatalf("A3 offer rejected: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE applications SET status = 'cancelled' WHERE id = $1`, f.app["A3"]); err != nil {
		t.Fatalf("A3 cancelled: %v", err)
	}
	return f
}

// create — оффер кредитора на заявку через сервис; ошибка роняет тест.
func (f *offersFixture) create(t *testing.T, lender, app string) lending.Offer {
	t.Helper()
	o, err := f.tryCreate(lender, app)
	if err != nil {
		t.Fatalf("create %s → %s: %v", lender, app, err)
	}
	return o
}

func (f *offersFixture) tryCreate(lender, app string) (lending.Offer, error) {
	return f.offers.Create(context.Background(), f.p[lender], service.ClientMeta{}, f.app[app], lending.CreateOfferInput{
		Amount:             decimal.NewFromInt(500000),
		InterestRateAnnual: decimal.NewFromInt(8),
		RepaymentSchedule:  "bullet",
	})
}

func assertCode(t *testing.T, what string, err error, want apperr.Code) {
	t.Helper()
	if got := apperr.CodeOf(err); got != want {
		t.Errorf("%s: code = %s (err %v), want %s", what, got, err, want)
	}
}

func TestOffersIntegration(t *testing.T) {
	ctx := context.Background()
	f := newOffersFixture(t)
	o1 := f.create(t, "L1", "A1")
	o2 := f.create(t, "L2", "A1")

	t.Run("create", func(t *testing.T) {
		if o1.Status != lending.OfferStatusSent || o1.Currency != "RUB" || o1.Lender.ID != f.p["L1"].ViewerOrg() {
			t.Errorf("o1 = %+v", o1)
		}
		var history int
		if err := f.tx.QueryRow(ctx, `SELECT count(*) FROM status_history
			WHERE entity_type = 'offer' AND entity_id = $1 AND from_status IS NULL AND to_status = 'sent'`,
			o1.ID).Scan(&history); err != nil || history != 1 {
			t.Errorf("status_history NULL → sent: %d, %v", history, err)
		}
		var notices int
		if err := f.tx.QueryRow(ctx, `SELECT count(*) FROM notifications
			WHERE user_id = $1 AND type = 'offer_received' AND entity_id = $2`,
			f.p["B1"].UserID, o1.ID.String()).Scan(&notices); err != nil || notices != 1 {
			t.Errorf("offer_received заёмщику: %d, %v", notices, err)
		}
	})

	t.Run("get visibility", func(t *testing.T) {
		tests := []struct {
			viewer string
			id     uuid.UUID
			want   bool
		}{
			{"L1", o1.ID, true}, {"L1", o2.ID, false},
			{"L2", o2.ID, true}, {"L2", o1.ID, false},
			{"B1", o1.ID, true}, {"B1", o2.ID, true},
			{"B2", o1.ID, false},
			{"admin", o1.ID, true},
		}
		for _, tt := range tests {
			got, err := f.offers.Get(ctx, f.p[tt.viewer], tt.id)
			switch {
			case tt.want && err != nil:
				t.Errorf("%s get %s: %v", tt.viewer, tt.id, err)
			case tt.want && got.ID != tt.id:
				t.Errorf("%s get %s вернул %s", tt.viewer, tt.id, got.ID)
			case !tt.want:
				assertCode(t, tt.viewer+" get чужого", err, apperr.CodeNotFound)
			}
		}
	})

	t.Run("list application offers", func(t *testing.T) {
		for viewer, want := range map[string]int{"B1": 2, "L1": 1, "L2": 1, "admin": 2} {
			got, err := f.offers.ListByApplication(ctx, f.p[viewer], f.app["A1"], nil)
			if err != nil {
				t.Fatalf("%s: %v", viewer, err)
			}
			if len(got) != want {
				t.Errorf("%s: %d офферов, want %d", viewer, len(got), want)
			}
		}
		// Кредитор видит только свой оффер, а не ставки конкурентов (DECISIONS п. 11).
		got, _ := f.offers.ListByApplication(ctx, f.p["L1"], f.app["A1"], nil)
		if len(got) == 1 && got[0].ID != o1.ID {
			t.Errorf("L1 видит чужой оффер %s", got[0].ID)
		}
		_, err := f.offers.ListByApplication(ctx, f.p["B2"], f.app["A1"], nil)
		assertCode(t, "B2 на чужую заявку", err, apperr.CodeNotFound)
	})

	t.Run("list offers", func(t *testing.T) {
		// Компании фикстуры новые, поэтому total считает только их офферы, без демо-сида.
		for viewer, want := range map[string]int64{"L1": 2, "L2": 1, "B1": 3, "B2": 0} {
			page, err := f.offers.List(ctx, f.p[viewer], lending.OfferFilter{})
			if err != nil {
				t.Fatalf("%s: %v", viewer, err)
			}
			if page.Total != want || int64(len(page.Items)) != want {
				t.Errorf("%s: total = %d, items = %d, want %d", viewer, page.Total, len(page.Items), want)
			}
		}
		nobody := service.Principal{UserID: uuid.New(), Role: "lender"}
		page, err := f.offers.List(ctx, nobody, lending.OfferFilter{})
		if err != nil || page.Total != 0 {
			t.Errorf("кредитор без организации: total = %d, %v", page.Total, err)
		}
	})

	t.Run("unread messages", func(t *testing.T) {
		lender, borrower := f.p["L1"], f.p["B1"]
		var first int64
		for i := range 3 {
			m, err := f.q.CreateMessage(ctx, repository.CreateMessageParams{
				OfferID: o1.ID, AuthorUserID: lender.UserID, Text: "сообщение",
			})
			if err != nil {
				t.Fatalf("message %d: %v", i, err)
			}
			if i == 0 {
				first = m.ID
			}
		}
		if err := f.q.MarkMessagesRead(ctx, repository.MarkMessagesReadParams{
			OfferID: o1.ID, UserID: borrower.UserID, LastReadMessageID: first,
		}); err != nil {
			t.Fatalf("mark read: %v", err)
		}

		// Свои сообщения не непрочитанные: у автора-кредитора 0 без строки message_reads.
		for name, want := range map[string]int64{"B1": 2, "L1": 0} {
			p := f.p[name]
			got, err := f.offers.Get(ctx, p, o1.ID)
			if err != nil {
				t.Fatalf("%s get: %v", name, err)
			}
			assertUnread(t, name+" get", got.UnreadMessages, want)

			page, err := f.offers.List(ctx, p, lending.OfferFilter{ApplicationID: &o1.ApplicationID})
			if err != nil {
				t.Fatalf("%s list: %v", name, err)
			}
			assertUnread(t, name+" list", findOffer(page.Items, o1.ID), want)

			items, err := f.offers.ListByApplication(ctx, p, o1.ApplicationID, nil)
			if err != nil {
				t.Fatalf("%s list by application: %v", name, err)
			}
			assertUnread(t, name+" list by application", findOffer(items, o1.ID), want)
		}
		got, err := f.offers.Get(ctx, f.p["admin"], o1.ID)
		if err != nil || got.UnreadMessages != nil {
			t.Errorf("admin: unread = %v, %v, want nil", got.UnreadMessages, err)
		}
	})

	t.Run("double offer", func(t *testing.T) {
		_, err := f.tryCreate("L1", "A1")
		assertCode(t, "второй активный оффер", err, apperr.CodeConflict)

		withdrawn, err := f.offers.Withdraw(ctx, f.p["L1"], service.ClientMeta{}, o1.ID)
		if err != nil {
			t.Fatalf("withdraw: %v", err)
		}
		if withdrawn.Status != lending.OfferStatusWithdrawn || withdrawn.DecidedAt == nil {
			t.Errorf("withdrawn = %+v", withdrawn)
		}
		if _, err := f.tryCreate("L1", "A1"); err != nil {
			t.Errorf("оффер после withdraw: %v", err)
		}
	})

	t.Run("reject", func(t *testing.T) {
		// Чужой заёмщик не видит оффер — 404 раньше проверки владельца в UPDATE.
		_, err := f.offers.Reject(ctx, f.p["B2"], service.ClientMeta{}, o2.ID, nil)
		assertCode(t, "B2 reject", err, apperr.CodeNotFound)

		comment := "дорого"
		got, err := f.offers.Reject(ctx, f.p["B1"], service.ClientMeta{}, o2.ID, &comment)
		if err != nil {
			t.Fatalf("reject: %v", err)
		}
		if got.Status != lending.OfferStatusRejected || got.RejectReason == nil ||
			*got.RejectReason != "rejected_by_borrower" || got.DecidedAt == nil ||
			got.RejectComment == nil || *got.RejectComment != comment {
			t.Errorf("rejected = %+v", got)
		}
		var history int
		if err := f.tx.QueryRow(ctx, `SELECT count(*) FROM status_history
			WHERE entity_type = 'offer' AND entity_id = $1 AND from_status = 'sent' AND to_status = 'rejected'
			  AND actor_user_id = $2 AND comment = $3`,
			o2.ID, f.p["B1"].UserID, comment).Scan(&history); err != nil || history != 1 {
			t.Errorf("status_history sent → rejected: %d, %v", history, err)
		}
		var notices int
		if err := f.tx.QueryRow(ctx, `SELECT count(*) FROM notifications
			WHERE user_id = $1 AND type = 'offer_rejected' AND entity_id = $2`,
			f.p["L2"].UserID, o2.ID.String()).Scan(&notices); err != nil || notices != 1 {
			t.Errorf("offer_rejected кредитору: %d, %v", notices, err)
		}
	})

	t.Run("create on unavailable application", func(t *testing.T) {
		// A3 видна L1 по его офферу, но cancelled — 409; чужой draft A2 не виден — 404.
		_, err := f.tryCreate("L1", "A3")
		assertCode(t, "cancelled", err, apperr.CodeConflict)
		_, err = f.tryCreate("L1", "A2")
		assertCode(t, "чужой draft", err, apperr.CodeNotFound)
	})
}

func assertUnread(t *testing.T, what string, got *int64, want int64) {
	t.Helper()
	switch {
	case got == nil:
		t.Errorf("%s: unread = nil, want %d", what, want)
	case *got != want:
		t.Errorf("%s: unread = %d, want %d", what, *got, want)
	}
}

// findOffer — unread_messages оффера id из списка; nil, если его там нет.
func findOffer(items []lending.Offer, id uuid.UUID) *int64 {
	for _, o := range items {
		if o.ID == id {
			return o.UnreadMessages
		}
	}
	return nil
}

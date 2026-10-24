package service_test

// Интеграционный тест правил видимости (IDOR): запросы *Visible с viewer_role/viewer_org
// на настоящем Postgres. Всё в одной транзакции с Rollback — демо-сид не меняется.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// listLimit больше, чем строк в демо-сиде: id фикстуры не отрезаются пагинацией.
// Если сид дорастёт до limit, проверка Count == len(List) у admin упадёт ложно.
const listLimit = 1000

type viewer struct {
	name string
	role repository.UserRole
	org  uuid.UUID
}

// visibilityFixture — id сущностей фикстуры по именам из плана BE-04.
type visibilityFixture map[string]uuid.UUID

// id — id по имени; опечатка в имени роняет тест, а не подставляет uuid.Nil.
func (fx visibilityFixture) id(t *testing.T, name string) uuid.UUID {
	t.Helper()
	id, ok := fx[name]
	if !ok {
		t.Fatalf("в фикстуре нет %q", name)
	}
	return id
}

// visibleEntity — запросы одной сущности; правило видимости задаётся отдельно, руками.
// get возвращает id найденной строки, чтобы тест сверил, что пришла запрошенная сущность.
type visibleEntity struct {
	notFoundMsg string
	get         func(ctx context.Context, q *repository.Queries, v viewer, id uuid.UUID) (uuid.UUID, error)
	list        func(ctx context.Context, q *repository.Queries, v viewer) ([]uuid.UUID, error)
	count       func(ctx context.Context, q *repository.Queries, v viewer) (int64, error)
}

// rowIDs — id строк списка в исходном порядке.
func rowIDs[T any](rows []T, id func(T) uuid.UUID) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = id(r)
	}
	return ids
}

var applicationsVisible = visibleEntity{
	notFoundMsg: "Заявка не найдена",
	get: func(ctx context.Context, q *repository.Queries, v viewer, id uuid.UUID) (uuid.UUID, error) {
		row, err := q.GetApplicationVisible(ctx, repository.GetApplicationVisibleParams{
			ID: id, ViewerRole: v.role, ViewerOrg: v.org,
		})
		return row.Application.ID, err
	},
	list: func(ctx context.Context, q *repository.Queries, v viewer) ([]uuid.UUID, error) {
		rows, err := q.ListApplicationsVisible(ctx, repository.ListApplicationsVisibleParams{
			PageLimit: listLimit, ViewerRole: v.role, ViewerOrg: v.org,
		})
		return rowIDs(rows, func(r repository.ListApplicationsVisibleRow) uuid.UUID { return r.Application.ID }), err
	},
	count: func(ctx context.Context, q *repository.Queries, v viewer) (int64, error) {
		return q.CountApplicationsVisible(ctx, repository.CountApplicationsVisibleParams{
			ViewerRole: v.role, ViewerOrg: v.org,
		})
	},
}

var offersVisible = visibleEntity{
	notFoundMsg: "Оффер не найден",
	get: func(ctx context.Context, q *repository.Queries, v viewer, id uuid.UUID) (uuid.UUID, error) {
		row, err := q.GetOfferVisible(ctx, repository.GetOfferVisibleParams{
			ID: id, ViewerRole: v.role, ViewerOrg: v.org,
		})
		return row.Offer.ID, err
	},
	list: func(ctx context.Context, q *repository.Queries, v viewer) ([]uuid.UUID, error) {
		rows, err := q.ListOffersVisible(ctx, repository.ListOffersVisibleParams{
			PageLimit: listLimit, ViewerRole: v.role, ViewerOrg: v.org,
		})
		return rowIDs(rows, func(r repository.ListOffersVisibleRow) uuid.UUID { return r.Offer.ID }), err
	},
	count: func(ctx context.Context, q *repository.Queries, v viewer) (int64, error) {
		return q.CountOffersVisible(ctx, repository.CountOffersVisibleParams{
			ViewerRole: v.role, ViewerOrg: v.org,
		})
	},
}

var dealsVisible = visibleEntity{
	notFoundMsg: "Сделка не найдена",
	get: func(ctx context.Context, q *repository.Queries, v viewer, id uuid.UUID) (uuid.UUID, error) {
		row, err := q.GetDealVisible(ctx, repository.GetDealVisibleParams{
			ID: id, ViewerRole: v.role, ViewerOrg: v.org,
		})
		return row.ID, err
	},
	list: func(ctx context.Context, q *repository.Queries, v viewer) ([]uuid.UUID, error) {
		rows, err := q.ListDealsVisible(ctx, repository.ListDealsVisibleParams{
			Limit: listLimit, ViewerRole: v.role, ViewerOrg: v.org,
		})
		return rowIDs(rows, func(r repository.Deal) uuid.UUID { return r.ID }), err
	},
	count: func(ctx context.Context, q *repository.Queries, v viewer) (int64, error) {
		return q.CountDealsVisible(ctx, repository.CountDealsVisibleParams{
			ViewerRole: v.role, ViewerOrg: v.org,
		})
	},
}

func TestVisibilityIntegration(t *testing.T) {
	ctx := context.Background()
	tx := beginTestTx(t)

	fx := seedVisibilityFixture(ctx, t, tx)
	q := repository.New(tx)

	viewers := []viewer{
		{"B1", repository.UserRoleBorrower, fx.id(t, "B1")},
		{"B2", repository.UserRoleBorrower, fx.id(t, "B2")},
		{"L1", repository.UserRoleLender, fx.id(t, "L1")},
		{"L2", repository.UserRoleLender, fx.id(t, "L2")},
		// У admin нет организации: viewer_org = uuid.Nil, ветка admin его не читает.
		{"admin", repository.UserRoleAdmin, uuid.Nil},
	}

	// Ожидания выписаны из правил видимости, а не из SQL: кто какие сущности фикстуры видит.
	// Все остальные сущности той же таблицы из фикстуры зритель не видит.
	tests := []struct {
		name    string
		entity  visibleEntity
		all     string
		visible map[string]string
	}{
		{
			name:   "applications",
			entity: applicationsVisible,
			all:    "A1 A2 A3 A4",
			visible: map[string]string{
				"B1":    "A1 A2",       // свои, любой статус
				"B2":    "A3 A4",       // свои
				"L1":    "A1 A3 A4",    // published + A4 по своему офферу O3
				"L2":    "A1 A3",       // только published
				"admin": "A1 A2 A3 A4", // все
			},
		},
		{
			name:   "offers",
			entity: offersVisible,
			all:    "O1 O2 O3",
			visible: map[string]string{
				"B1":    "O1 O2",    // офферы на свои заявки
				"B2":    "O3",       // офферы на свои заявки
				"L1":    "O1 O3",    // свои
				"L2":    "O2",       // свои
				"admin": "O1 O2 O3", // все
			},
		},
		{
			name:   "deals",
			entity: dealsVisible,
			all:    "D1",
			visible: map[string]string{
				"B2":    "D1", // сторона сделки
				"L1":    "D1", // сторона сделки
				"admin": "D1",
			},
		},
	}

	viewerNames := make([]string, len(viewers))
	for i, v := range viewers {
		viewerNames[i] = v.name
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			all := strings.Fields(tt.all)
			// Опечатка в таблице ожиданий иначе тихо превратилась бы в «не видит».
			for name, entities := range tt.visible {
				if !slices.Contains(viewerNames, name) {
					t.Fatalf("в ожиданиях неизвестный зритель %q", name)
				}
				for _, e := range strings.Fields(entities) {
					if !slices.Contains(all, e) {
						t.Fatalf("%s: в ожиданиях сущность %q вне %q", name, e, tt.all)
					}
				}
			}
			for _, v := range viewers {
				visible := strings.Fields(tt.visible[v.name])
				listed, err := tt.entity.list(ctx, q, v)
				if err != nil {
					t.Fatalf("%s list: %v", v.name, err)
				}
				count, err := tt.entity.count(ctx, q, v)
				if err != nil {
					t.Fatalf("%s count: %v", v.name, err)
				}
				// Блоки видимости в List и Count копируются руками: расхождение ловится здесь.
				if count != int64(len(listed)) {
					t.Errorf("%s: count = %d, len(list) = %d", v.name, count, len(listed))
				}

				for _, name := range all {
					id := fx.id(t, name)
					want := slices.Contains(visible, name)
					if got := slices.Contains(listed, id); got != want {
						t.Errorf("%s: %s в списке = %v, want %v", v.name, name, got, want)
					}
					gotID, err := tt.entity.get(ctx, q, v, id)
					switch {
					case want && err != nil:
						t.Errorf("%s: get %s: %v", v.name, name, err)
					case want && gotID != id:
						t.Errorf("%s: get %s вернул id %s", v.name, name, gotID)
					case !want:
						assertNotFound(t, tt.entity, err, v.name+" get "+name)
					}
				}
				// Чужое неотличимо от несуществующего: тот же NOT_FOUND.
				_, err = tt.entity.get(ctx, q, v, uuid.New())
				assertNotFound(t, tt.entity, err, v.name+" get случайного id")
			}

			// Закрыто по умолчанию: роль без организации не видит ничего.
			nobody := viewer{"borrower без организации", repository.UserRoleBorrower, uuid.Nil}
			listed, err := tt.entity.list(ctx, q, nobody)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(listed) != 0 {
				t.Errorf("%s видит %d строк, want 0", nobody.name, len(listed))
			}
		})
	}
}

func assertNotFound(t *testing.T, e visibleEntity, err error, what string) {
	t.Helper()
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("%s: err = %v, want pgx.ErrNoRows", what, err)
		return
	}
	if code := apperr.CodeOf(pgerr.Map(err, e.notFoundMsg)); code != apperr.CodeNotFound {
		t.Errorf("%s: pgerr.Map code = %s, want %s", what, code, apperr.CodeNotFound)
	}
}

// seedVisibilityFixture создаёт фикстуру плана BE-04:
//   - организации: заёмщики B1, B2, кредиторы L1, L2, у каждой по пользователю;
//   - заявки: A1 (B1, published), A2 (B1, draft), A3 (B2, published), A4 (B2, agreed);
//   - офферы: O1 (L1 → A1, sent), O2 (L2 → A1, sent), O3 (L1 → A4, accepted);
//   - сделка D1 (A4/O3, B2–L1).
//
// Порядок задают CHECK: A4 создаётся published, O3 — sent; затем O3 → accepted с decided_at,
// A4 → agreed с accepted_offer_id (applications_agreed_chk, offers_decided_chk).
func seedVisibilityFixture(ctx context.Context, t *testing.T, tx pgx.Tx) visibilityFixture {
	t.Helper()
	fx := visibilityFixture{}
	insert := func(name, sql string, args ...any) {
		t.Helper()
		var id uuid.UUID
		if err := tx.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("fixture %s: %v", name, err)
		}
		fx[name] = id
	}
	exec := func(what, sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %s: %v", what, err)
		}
	}

	orgs := []struct {
		name    string
		role    repository.UserRole
		country string
	}{
		{"B1", repository.UserRoleBorrower, "RU"}, {"B2", repository.UserRoleBorrower, "RU"},
		{"L1", repository.UserRoleLender, "AE"}, {"L2", repository.UserRoleLender, "CN"},
	}
	for _, o := range orgs {
		insert(o.name, `INSERT INTO organizations (type, name, country_code, registration_number, legal_address)
			VALUES ($1, $2, $3, $4, 'Адрес') RETURNING id`,
			o.role, "Visibility "+o.name, o.country, uniqueRegistrationNumber())
		orgID := fx.id(t, o.name)
		fx["user"+o.name] = insertTestUser(ctx, t, tx, o.role, &orgID)
	}

	apps := []struct{ name, org, status string }{
		{"A1", "B1", "published"}, {"A2", "B1", "draft"},
		{"A3", "B2", "published"}, {"A4", "B2", "published"},
	}
	for _, a := range apps {
		insert(a.name, `INSERT INTO applications (borrower_organization_id, created_by_user_id, status,
				amount, currency, target_country_code, purpose, term_months, transfer_method)
			VALUES ($1, $2, $3, 1000000, 'RUB', 'AE', 'Пополнение оборотных средств', 12, 'swift')
			RETURNING id`,
			fx.id(t, a.org), fx.id(t, "user"+a.org), a.status)
	}

	offers := []struct{ name, lender, app string }{
		{"O1", "L1", "A1"}, {"O2", "L2", "A1"}, {"O3", "L1", "A4"},
	}
	for _, o := range offers {
		insert(o.name, `INSERT INTO offers (application_id, lender_organization_id, created_by_user_id,
				amount, currency, interest_rate_annual, repayment_schedule)
			VALUES ($1, $2, $3, 1000000, 'RUB', 8, 'bullet') RETURNING id`,
			fx.id(t, o.app), fx.id(t, o.lender), fx.id(t, "user"+o.lender))
	}
	exec("O3 accepted", `UPDATE offers SET status = 'accepted', decided_at = now() WHERE id = $1`, fx.id(t, "O3"))
	exec("A4 agreed", `UPDATE applications SET status = 'agreed', accepted_offer_id = $2 WHERE id = $1`,
		fx.id(t, "A4"), fx.id(t, "O3"))

	insert("D1", `INSERT INTO deals (application_id, offer_id, borrower_organization_id, lender_organization_id,
			amount, currency, interest_rate_annual, term_months, target_country_code, transfer_method,
			repayment_schedule)
		VALUES ($1, $2, $3, $4, 1000000, 'RUB', 8, 12, 'AE', 'swift', 'bullet') RETURNING id`,
		fx.id(t, "A4"), fx.id(t, "O3"), fx.id(t, "B2"), fx.id(t, "L1"))
	return fx
}

package servicetest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanupCommittedOrgs берёт пул владельца схемы сразу (без MIGRATE_DATABASE_URL тест
// пропускается до того, как что-то закоммитит) и возвращает функцию регистрации: каждая
// компания, переданная в неё, удаляется в t.Cleanup со всеми своими данными (DeleteCommittedOrgs).
// Регистрировать компанию сразу после создания: так очистка сработает и при t.Fatal в середине.
func CleanupCommittedOrgs(t *testing.T) (track func(orgID uuid.UUID)) {
	t.Helper()
	owner := NewOwnerPool(t)
	var orgs []uuid.UUID
	t.Cleanup(func() { DeleteCommittedOrgs(t, owner, orgs...) })
	return func(orgID uuid.UUID) { orgs = append(orgs, orgID) }
}

// DeleteCommittedOrgs удаляет закоммиченные данные компаний orgs (заявки, офферы, сделки, чат,
// документы, история, аудит, пользователи) в порядке внешних ключей. pool — владельца схемы
// (NewOwnerPool): у lp_app нет DELETE, а без чистки каждый прогон оставлял бы мусор в dev-БД.
func DeleteCommittedOrgs(t *testing.T, pool *pgxpool.Pool, orgs ...uuid.UUID) {
	t.Helper()
	if len(orgs) == 0 {
		return
	}
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Errorf("cleanup begin: %v", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, q := range []string{
		`DELETE FROM status_history WHERE entity_id = ANY($1) OR entity_id IN (
			SELECT id FROM applications WHERE borrower_organization_id = ANY($1)
			UNION SELECT id FROM offers WHERE lender_organization_id = ANY($1)
			UNION SELECT id FROM deals WHERE borrower_organization_id = ANY($1) OR lender_organization_id = ANY($1))
		   OR actor_user_id IN (SELECT id FROM users WHERE organization_id = ANY($1))`,
		`DELETE FROM audit_log WHERE user_id IN (SELECT id FROM users WHERE organization_id = ANY($1))`,
		`DELETE FROM deals WHERE borrower_organization_id = ANY($1) OR lender_organization_id = ANY($1)`,
		`DELETE FROM messages WHERE author_user_id IN (SELECT id FROM users WHERE organization_id = ANY($1))
		   OR offer_id IN (SELECT id FROM offers WHERE lender_organization_id = ANY($1)
		       OR application_id IN (SELECT id FROM applications WHERE borrower_organization_id = ANY($1)))`,
		// Заявка и оффер ссылаются друг на друга (accepted_offer_id), а agreed без него не
		// пускает applications_agreed_chk: удаляем одной командой, FK проверятся в её конце.
		`WITH o AS (DELETE FROM offers WHERE lender_organization_id = ANY($1)
		    OR application_id IN (SELECT id FROM applications WHERE borrower_organization_id = ANY($1)))
		 DELETE FROM applications WHERE borrower_organization_id = ANY($1)`,
		`DELETE FROM kyc_documents WHERE organization_id = ANY($1)`, // uploaded_by_user_id — без CASCADE
		`DELETE FROM users WHERE organization_id = ANY($1)`,         // sessions, notifications, message_reads — CASCADE
		`DELETE FROM organizations WHERE id = ANY($1)`,              // beneficiaries, registry_checks, aml_flags — CASCADE
	} {
		if _, err := tx.Exec(ctx, q, orgs); err != nil {
			t.Errorf("cleanup: %v; %s", err, q)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Errorf("cleanup commit: %v", err)
	}
}

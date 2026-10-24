package service_test

// Интеграционный тест Notifier на настоящем Postgres: состав получателей в INSERT … SELECT
// и атомарность с событием (ADR-0001). Всё в одной транзакции с откатом в Cleanup;
// InTx открывает внутри savepoint, его откат и проверяется.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// failingOrgWriter — настоящий querier, у которого падает CreateNotificationsForOrganization.
type failingOrgWriter struct {
	repository.Querier
	err error
}

func (w failingOrgWriter) CreateNotificationsForOrganization(context.Context, repository.CreateNotificationsForOrganizationParams) (int64, error) {
	return 0, w.err
}

func TestNotifierIntegration(t *testing.T) {
	ctx := context.Background()
	tx := beginTestTx(t)
	q := repository.New(tx)
	notifier := service.Notifier{}

	countFor := func(t *testing.T, userID uuid.UUID, entityID string) int {
		t.Helper()
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND entity_id = $2`,
			userID, entityID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	notification := func() service.Notification {
		return service.Notification{
			Type: repository.NotificationTypeKycSubmitted, Title: "KYC на проверке",
			EntityType: repository.EntityTypeOrganization, EntityID: uuid.NewString(),
		}
	}

	t.Run("NotifyAdmins — каждому активному админу, заблокированному нет", func(t *testing.T) {
		active1 := insertTestUser(ctx, t, tx, repository.UserRoleAdmin, nil)
		active2 := insertTestUser(ctx, t, tx, repository.UserRoleAdmin, nil)
		blocked := insertTestUser(ctx, t, tx, repository.UserRoleAdmin, nil)
		if _, err := q.BlockUser(ctx, repository.BlockUserParams{ID: blocked, BlockedReason: pgtype.Text{String: "тест", Valid: true}}); err != nil {
			t.Fatal(err)
		}
		n := notification()

		if err := notifier.NotifyAdmins(ctx, q, n); err != nil {
			t.Fatal(err)
		}

		if a1, a2, b := countFor(t, active1, n.EntityID), countFor(t, active2, n.EntityID), countFor(t, blocked, n.EntityID); a1 != 1 || a2 != 1 || b != 0 {
			t.Fatalf("active1 %d, active2 %d, blocked %d", a1, a2, b)
		}
	})

	t.Run("NotifyOrganization — пользователю компании, другой компании нет", func(t *testing.T) {
		org := insertDealsOrg(ctx, t, tx, repository.UserRoleBorrower, "Notifier Org")
		other := insertDealsOrg(ctx, t, tx, repository.UserRoleLender, "Notifier Other")
		n := notification()

		if err := notifier.NotifyOrganization(ctx, q, *org.OrgID, n); err != nil {
			t.Fatal(err)
		}

		if got, otherGot := countFor(t, org.UserID, n.EntityID), countFor(t, other.UserID, n.EntityID); got != 1 || otherGot != 0 {
			t.Fatalf("org user %d, other %d", got, otherGot)
		}
	})

	t.Run("ошибка уведомления в InTx откатывает событие и прежние уведомления", func(t *testing.T) {
		admin := insertTestUser(ctx, t, tx, repository.UserRoleAdmin, nil)
		org := insertDealsOrg(ctx, t, tx, repository.UserRoleBorrower, "Notifier Rollback")
		n := notification()
		dbErr := errors.New("insert failed")

		err := service.NewTxManager(tx).InTx(ctx, func(q repository.Querier) error {
			// Событие — блокировка пользователя; затем два уведомления, второе падает.
			if _, err := q.BlockUser(ctx, repository.BlockUserParams{ID: org.UserID, BlockedReason: pgtype.Text{String: "тест", Valid: true}}); err != nil {
				return err
			}
			if err := notifier.NotifyAdmins(ctx, q, n); err != nil {
				return err
			}
			return notifier.NotifyOrganization(ctx, failingOrgWriter{Querier: q, err: dbErr}, *org.OrgID, n)
		})

		if !errors.Is(err, dbErr) {
			t.Fatalf("err = %v, want %v", err, dbErr)
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, org.UserID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "active" || countFor(t, admin, n.EntityID) != 0 {
			t.Fatalf("status %s, admin notifications %d — откат не произошёл", status, countFor(t, admin, n.EntityID))
		}
	})
}

package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/pgconv"
)

// Действия audit_log блокировки пользователей.
const (
	AuditUserBlock   = "user.block"
	AuditUserUnblock = "user.unblock"
)

const userNotFound = "Пользователь не найден"

// User — пользователь в ответах админки: только поля User контракта, без password_hash.
type User struct {
	ID             uuid.UUID
	Email          string
	Role           string
	Status         string
	OrganizationID *uuid.UUID // nil у admin
	CreatedAt      time.Time
}

// UserFilter — фильтры списка пользователей. nil — фильтр не задан.
type UserFilter struct {
	Role     *string
	Status   *string
	Q        *string // подстрока email
	Page     int
	PageSize int
}

// UserPage — страница пользователей.
type UserPage struct {
	Items    []User
	Total    int64
	Page     int
	PageSize int
}

// ListUsers — пользователи платформы с фильтрами и поиском по email.
func (a *Admin) ListUsers(ctx context.Context, p service.Principal, f UserFilter) (UserPage, error) {
	if err := requireAdmin(p); err != nil {
		return UserPage{}, err
	}
	page, size, limit, offset := service.PageWindow(f.Page, f.PageSize)
	count := repository.CountUsersParams{Q: likeQuery(f.Q)}
	if f.Role != nil {
		count.Role = repository.NullUserRole{UserRole: repository.UserRole(*f.Role), Valid: true}
	}
	if f.Status != nil {
		count.Status = repository.NullUserStatus{UserStatus: repository.UserStatus(*f.Status), Valid: true}
	}
	rows, err := a.store.ListUsers(ctx, repository.ListUsersParams{
		Limit: limit, Offset: offset, Role: count.Role, Status: count.Status, Q: count.Q,
	})
	if err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	total, err := a.store.CountUsers(ctx, count)
	if err != nil {
		return UserPage{}, fmt.Errorf("count users: %w", err)
	}
	items := make([]User, len(rows))
	for i, r := range rows {
		items[i] = userFromListRow(r)
	}
	return UserPage{Items: items, Total: total, Page: page, PageSize: size}, nil
}

// BlockUser блокирует пользователя и удаляет все его сессии.
//
// Доступ теряется сразу и без удаления сессий: GetSessionByTokenHash требует
// status = 'active'. Удаление — чистка (требование issue), а не единственная защита.
// Повторный block идемпотентен: 200 без новых записей. Уведомления нет: для блокировки нет
// NotificationType, а заблокированный его всё равно не увидит.
func (a *Admin) BlockUser(ctx context.Context, p service.Principal, userID uuid.UUID, reason string, meta service.ClientMeta) (User, error) {
	return a.setUserStatus(ctx, p, meta, userID, userStatusChange{
		to:     repository.UserStatusBlocked,
		audit:  AuditUserBlock,
		reason: reason,
		apply: func(ctx context.Context, q repository.Querier) error {
			if _, err := q.BlockUser(ctx, repository.BlockUserParams{
				ID: userID, BlockedReason: pgtype.Text{String: reason, Valid: true},
			}); err != nil {
				return fmt.Errorf("block user: %w", err)
			}
			if err := q.DeleteSessionsByUserID(ctx, userID); err != nil {
				return fmt.Errorf("delete sessions: %w", err)
			}
			return nil
		},
	})
}

// UnblockUser снимает блокировку. Сессии не восстанавливаются: пользователь входит заново.
func (a *Admin) UnblockUser(ctx context.Context, p service.Principal, userID uuid.UUID, meta service.ClientMeta) (User, error) {
	return a.setUserStatus(ctx, p, meta, userID, userStatusChange{
		to:    repository.UserStatusActive,
		audit: AuditUserUnblock,
		apply: func(ctx context.Context, q repository.Querier) error {
			if _, err := q.UnblockUser(ctx, userID); err != nil {
				return fmt.Errorf("unblock user: %w", err)
			}
			return nil
		},
	})
}

// userStatusChange — то, чем block и unblock отличаются; порядок шагов — в setUserStatus.
type userStatusChange struct {
	to     repository.UserStatus
	audit  string
	reason string // "" — без причины (unblock)
	apply  func(ctx context.Context, q repository.Querier) error
}

// setUserStatus — смена статуса пользователя в одной транзакции под FOR UPDATE: два
// параллельных block не запишут историю дважды.
func (a *Admin) setUserStatus(ctx context.Context, p service.Principal, meta service.ClientMeta, userID uuid.UUID, c userStatusChange) (User, error) {
	if err := requireAdmin(p); err != nil {
		return User{}, err
	}
	var out User
	err := a.tx.InTx(ctx, func(q repository.Querier) error {
		u, err := q.GetUserForUpdate(ctx, userID)
		if err != nil {
			return pgerr.Map(err, userNotFound)
		}
		// Админа (и себя) не блокируем: иначе платформа может остаться без администратора.
		if u.Role == repository.UserRoleAdmin {
			return apperr.Forbidden("Нельзя заблокировать администратора")
		}
		if u.Status == c.to {
			out = userFromRepository(u)
			return nil
		}
		if err := c.apply(ctx, q); err != nil {
			return err
		}
		if err := service.RecordStatusChange(ctx, q, service.StatusChange{
			EntityType: repository.EntityTypeUser, EntityID: userID,
			From: string(u.Status), To: string(c.to), Actor: &p, Comment: c.reason,
		}); err != nil {
			return err
		}
		var auditMeta map[string]any
		if c.reason != "" {
			auditMeta = map[string]any{"reason": c.reason}
		}
		if err := service.WriteAudit(ctx, q, meta, service.AuditEvent{
			Action: c.audit, UserID: &p.UserID, Meta: auditMeta,
			EntityType: repository.EntityTypeUser, EntityID: userID.String(),
		}); err != nil {
			return err
		}
		u.Status = c.to
		out = userFromRepository(u)
		return nil
	})
	return out, err
}

func userFromRepository(u repository.User) User {
	return User{
		ID: u.ID, Email: u.Email, Role: string(u.Role), Status: string(u.Status),
		OrganizationID: pgconv.UUIDPtr(u.OrganizationID), CreatedAt: u.CreatedAt.Time,
	}
}

func userFromListRow(u repository.ListUsersRow) User {
	return User{
		ID: u.ID, Email: u.Email, Role: string(u.Role), Status: string(u.Status),
		OrganizationID: pgconv.UUIDPtr(u.OrganizationID), CreatedAt: u.CreatedAt.Time,
	}
}

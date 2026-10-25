// Package pgerr переводит ошибки Postgres/pgx в доменные ошибки apperr.
//
// Используется в service поверх вызовов sqlc:
//
//	app, err := q.GetApplicationVisible(ctx, repository.GetApplicationVisibleParams{
//		ID: id, ViewerRole: p.ViewerRole(), ViewerOrg: p.ViewerOrg(),
//	})
//	if err != nil {
//		return nil, pgerr.Map(err, "Заявка не найдена")
//	}
package pgerr

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

// Коды SQLSTATE, которые имеют смысл для клиента.
const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
	checkViolation      = "23514"
)

// Временные отказы: таймауты роли lp_app (backend/db/roles.sql), лимит соединений роли и
// конфликты конкурентных транзакций. Повтор запроса обычно проходит.
var transientCodes = map[string]bool{
	"57014": true, // query_canceled — statement_timeout
	"55P03": true, // lock_not_available — lock_timeout
	"25P03": true, // idle_in_transaction_session_timeout
	"53300": true, // too_many_connections — CONNECTION LIMIT роли
	"40001": true, // serialization_failure
	"40P01": true, // deadlock_detected
}

const transientMessage = "Сервис временно перегружен, повторите запрос"

// Map: pgx.ErrNoRows → NOT_FOUND (с notFoundMsg), unique → CONFLICT,
// foreign key / check → VALIDATION_ERROR, временные отказы (Transient) → UNAVAILABLE.
// Остальное возвращается как есть (HTTP-слой отдаст INTERNAL и запишет причину в лог). nil → nil.
func Map(err error, notFoundMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, notFoundMsg, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case uniqueViolation:
			return apperr.Wrap(apperr.CodeConflict, "Запись уже существует", err)
		case foreignKeyViolation, checkViolation:
			return apperr.Wrap(apperr.CodeValidation, "Некорректные данные", err)
		}
	}
	return Transient(err)
}

// Transient превращает недоменную ошибку временного отказа БД (таймаут, лимит соединений,
// дедлок) в UNAVAILABLE (503): клиент может повторить запрос. Остальное — как есть.
// HTTP-слой вызывает его для всех ошибок: большинство вызовов sqlc идёт мимо Map.
func Transient(err error) error {
	if _, ok := apperr.As(err); ok {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && transientCodes[pgErr.Code] {
		return apperr.Unavailable(transientMessage, err)
	}
	return err
}

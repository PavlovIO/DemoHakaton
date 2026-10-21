// Package pgerr переводит ошибки Postgres/pgx в доменные ошибки apperr.
//
// Используется в service поверх вызовов sqlc:
//
//	app, err := q.GetApplicationByID(ctx, id)
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

// Map: pgx.ErrNoRows → NOT_FOUND (с notFoundMsg), unique → CONFLICT,
// foreign key / check → VALIDATION_ERROR. Остальное возвращается как есть
// (HTTP-слой отдаст INTERNAL и запишет причину в лог). nil → nil.
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
	return err
}

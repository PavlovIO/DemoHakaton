package service_test

// Интеграционный тест прав роли приложения (backend/db/roles.sql + миграция 00003_app_role_grants).
// DATABASE_URL должен указывать на lp_app: БД сама не даёт переписать audit_log, изменить сделку,
// выполнить DDL или команду ОС. Положенные права проверяют остальные интеграционные тесты пакета.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// sqlStateInsufficientPrivilege — SQLSTATE 42501 insufficient_privilege.
const sqlStateInsufficientPrivilege = "42501"

func TestDBPrivilegesIntegration(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()

	// Под суперпользователем запреты ниже не сработали бы, а часть команд выполнилась бы.
	var superuser bool
	if err := pool.QueryRow(ctx, "SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&superuser); err != nil {
		t.Fatalf("check role: %v", err)
	}
	if superuser {
		t.Fatal("DATABASE_URL подключается суперпользователем — обнови .env из .env.example")
	}

	forbidden := []struct {
		name string
		sql  string
	}{
		{"drop table", "DROP TABLE audit_log"},
		{"create table", "CREATE TABLE privileges_probe (id int)"},
		{"update audit_log", "UPDATE audit_log SET action = 'x'"},
		{"delete audit_log", "DELETE FROM audit_log"},
		{"update deals", "UPDATE deals SET fee_amount = 0"},
		{"insert countries", "INSERT INTO countries (code, name_ru, name_en) VALUES ('ZZ', 'x', 'x')"},
		{"truncate sessions", "TRUNCATE sessions"},
		{"copy to program", "COPY (SELECT 1) TO PROGRAM 'id'"},
		{"copy from program", "COPY countries FROM PROGRAM 'id'"},
	}
	for _, tc := range forbidden {
		t.Run(tc.name, func(t *testing.T) {
			// Каждый случай — в своей транзакции: ошибка не обрывает следующие, а если право
			// всё же есть, Rollback откатывает изменение.
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			_, err = tx.Exec(ctx, tc.sql)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != sqlStateInsufficientPrivilege {
				t.Errorf("%s: error = %v, want SQLSTATE %s", tc.sql, err, sqlStateInsufficientPrivilege)
			}
		})
	}

	t.Run("statement timeout", func(t *testing.T) {
		var timeout string
		if err := pool.QueryRow(ctx, "SHOW statement_timeout").Scan(&timeout); err != nil {
			t.Fatalf("show statement_timeout: %v", err)
		}
		if timeout != "5s" {
			t.Errorf("statement_timeout = %q, want 5s", timeout)
		}
	})

	// Новая таблица без GRANT для lp_app в своей миграции попадёт сюда.
	t.Run("every table is granted", func(t *testing.T) {
		rows, err := pool.Query(ctx, `
			SELECT tablename FROM pg_tables
			WHERE schemaname = 'public' AND tablename <> 'goose_db_version'
			  AND NOT has_table_privilege('lp_app', format('%I.%I', schemaname, tablename), 'SELECT')
			ORDER BY tablename`)
		if err != nil {
			t.Fatalf("query tables: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var table string
			if err := rows.Scan(&table); err != nil {
				t.Fatalf("scan: %v", err)
			}
			t.Errorf("table %s: no SELECT for lp_app — add GRANT in its migration", table)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
	})
}

// Команда loadsanctions загружает санкционный список в sanctions_entries (BE-13).
//
//	make load-sanctions                                               # мок-список
//	make load-sanctions SANCTIONS_SOURCE=uk_fcdo SANCTIONS_FILE=uk.csv # UK Sanctions List (CSV)
//
// Источник заменяется атомарно: DELETE … WHERE source и COPY новых строк в одной
// транзакции — частичной загрузки не бывает. Подключение — MIGRATE_DATABASE_URL (роль
// миграций, DB-SEC), без неё — DATABASE_URL: у lp_app на sanctions_entries только SELECT.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml/sanctionslist"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	source := flag.String("source", sanctionslist.SourceMock, "источник: mock | uk_fcdo")
	file := flag.String("file", "", "CSV UK Sanctions List (обязателен для uk_fcdo)")
	flag.Parse()

	if err := run(ctx, *source, *file); err != nil {
		slog.Error("load sanctions failed", "source", *source, "err", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, source, file string) error {
	entries, err := readEntries(source, file)
	if err != nil {
		return err
	}
	rows := toRows(source, entries)

	url := os.Getenv("MIGRATE_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		return errors.New("нужен MIGRATE_DATABASE_URL или DATABASE_URL")
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())

	var deleted, inserted int64
	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		q := repository.New(tx)
		if deleted, err = q.DeleteSanctionsBySource(ctx, source); err != nil {
			return fmt.Errorf("delete %s: %w", source, err)
		}
		if inserted, err = q.InsertSanctionsEntries(ctx, rows); err != nil {
			return fmt.Errorf("insert %s: %w", source, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("source=%s deleted=%d inserted=%d\n", source, deleted, inserted)
	return nil
}

func readEntries(source, file string) ([]sanctionslist.Entry, error) {
	switch source {
	case sanctionslist.SourceMock:
		return sanctionslist.Mock()
	case sanctionslist.SourceUKFCDO:
		if file == "" {
			return nil, errors.New("для uk_fcdo нужен -file (CSV со страницы gov.uk «The UK Sanctions List»)")
		}
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return sanctionslist.ParseUK(f)
	default:
		return nil, fmt.Errorf("неизвестный источник %q: mock | uk_fcdo", source)
	}
}

// toRows нормализует имена той же функцией, что скрининг (aml.Normalize), и снимает дубли
// по (external_id, name_normalized) — ключу UNIQUE таблицы: в UK-списке строка на каждый
// адрес, а алиасы после нормализации часто совпадают. Пустое после нормализации имя
// (одна орг. форма) не ищется — пропускается.
func toRows(source string, entries []sanctionslist.Entry) []repository.InsertSanctionsEntriesParams {
	type key struct{ externalID, normalized string }
	seen := make(map[key]bool, len(entries))
	rows := make([]repository.InsertSanctionsEntriesParams, 0, len(entries))
	for _, e := range entries {
		normalized := aml.Normalize(e.Name)
		k := key{e.ExternalID, normalized}
		if normalized == "" || seen[k] {
			continue
		}
		seen[k] = true
		row := repository.InsertSanctionsEntriesParams{
			Source: source, ExternalID: e.ExternalID, EntityKind: e.EntityKind,
			Name: e.Name, NameNormalized: normalized, Raw: e.Raw,
		}
		if e.CountryCode != nil {
			row.CountryCode = pgtype.Text{String: *e.CountryCode, Valid: true}
		}
		rows = append(rows, row)
	}
	return rows
}

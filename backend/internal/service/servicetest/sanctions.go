package servicetest

import (
	"context"
	"testing"
)

// RequireSanctionsMock падает, если мок-санкционный список не загружен. Не Skip: под lp_app
// тест вставить записи не может (DB-SEC), а пропуск дал бы зелёный CI без проверки.
func RequireSanctionsMock(ctx context.Context, t *testing.T, db RowQuerier) {
	t.Helper()
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM sanctions_entries WHERE source = 'mock'`).Scan(&n); err != nil {
		t.Fatalf("sanctions_entries: %v", err)
	}
	if n == 0 {
		t.Fatal("мок-санкционный список не загружен: запусти make load-sanctions")
	}
}

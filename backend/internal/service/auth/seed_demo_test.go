package auth_test

import (
	"context"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
)

// demoPassword — общий пароль демо-аккаунтов из README (BE-16, DECISIONS п. 30).
const demoPassword = "Demo-Pass-2026"

const seedCommonPath = "../../../migrations/testdata/seed_demo_common.sql"

// demoEmails — аккаунты из таблицы шага 1 плана BE-16.
var demoEmails = []string{
	"admin@liquidity.local", "alpha@liquidity.local", "beta@liquidity.local", "gamma@liquidity.local",
	"vector@liquidity.local", "delta@liquidity.local", "epsilon@liquidity.local", "omega@liquidity.local",
	"sigma@liquidity.local",
}

// seedUserRow — строка VALUES пользователя в сиде: '<email>', '<PHC-хэш>'.
var seedUserRow = regexp.MustCompile(`'([a-z]+@liquidity\.local)',\s*'(\$argon2id\$[^']+)'`)

// Хэш в сиде закоммичен один раз (генерация на каждом сиде требовала бы Go в make seed),
// поэтому соответствие паролю из README сторожит этот тест: иначе вход под демо-аккаунтом
// сломался бы молча, до первой попытки на демо.
func TestSeedDemoPasswords(t *testing.T) {
	sql, err := os.ReadFile(seedCommonPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sql), "PLACEHOLDER") {
		t.Error("seed contains PLACEHOLDER password hashes")
	}
	if err := auth.ValidatePassword(demoPassword); err != nil {
		t.Fatalf("ValidatePassword(%q) = %v", demoPassword, err)
	}

	rows := seedUserRow.FindAllStringSubmatch(string(sql), -1)
	var emails []string
	for _, row := range rows {
		email, hash := row[1], row[2]
		emails = append(emails, email)
		ok, err := auth.VerifyPassword(context.Background(), demoPassword, hash)
		if err != nil || !ok {
			t.Errorf("%s: VerifyPassword(%q) = %v, %v; want true", email, demoPassword, ok, err)
		}
	}
	slices.Sort(emails)
	want := slices.Clone(demoEmails)
	slices.Sort(want)
	if !slices.Equal(emails, want) {
		t.Errorf("seed users = %v, want %v", emails, want)
	}
}

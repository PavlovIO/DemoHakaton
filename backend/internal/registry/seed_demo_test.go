package registry_test

import (
	"context"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/registry"
)

const seedCommonPath = "../../migrations/testdata/seed_demo_common.sql"

// seedOrgRow — реквизиты строки VALUES организации в сиде: страна, рег. номер, LEI или NULL.
var seedOrgRow = regexp.MustCompile(`'(borrower|lender)', '[^']+', '([A-Z]{2})', '([^']+)', (?:'([A-Z0-9]{20})'|NULL)`)

// demoCompaniesInSeed — 8 компаний таблицы шага 1 плана BE-16.
const demoCompaniesInSeed = 8

// Реквизиты демо-компаний правдоподобны (DECISIONS п. 29), а мок-реестр знает каждую:
// иначе submitKyc на демо дал бы флаг «не найдена в реестре» и сломал US-14.
func TestSeedDemoIdentifiers(t *testing.T) {
	sql, err := os.ReadFile(seedCommonPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := seedOrgRow.FindAllStringSubmatch(string(sql), -1)
	if len(rows) != demoCompaniesInSeed {
		t.Fatalf("seed organizations = %d, want %d", len(rows), demoCompaniesInSeed)
	}
	mock := registry.NewMock()
	for _, row := range rows {
		country, regNumber, lei := row[2], row[3], row[4]
		lookupKey := lei
		if country == "RU" {
			lookupKey = regNumber
			if !innChecksumValid(regNumber) {
				t.Errorf("ИНН %s: неверная контрольная сумма", regNumber)
			}
		}
		if lei != "" && !leiChecksumValid(lei) {
			t.Errorf("LEI %s: неверная контрольная сумма", lei)
		}
		if lookupKey == "" {
			t.Errorf("%s %s: нет идентификатора для сверки с реестром", country, regNumber)
			continue
		}
		got, err := mock.Lookup(context.Background(), country, lookupKey)
		if err != nil || !got.Found {
			t.Errorf("mock registry %s/%s: found=%v err=%v; want fixture", country, lookupKey, got.Found, err)
			continue
		}
		if got.RegistrationNumber != regNumber {
			t.Errorf("mock registry %s/%s: registration_number %s, seed %s", country, lookupKey, got.RegistrationNumber, regNumber)
		}
	}
}

// Сами проверки — на реальных реквизитах: ИНН Сбербанка, LEI Bloomberg Finance L.P.
func TestChecksumHelpers(t *testing.T) {
	if !innChecksumValid("7707083893") || innChecksumValid("7707083894") || innChecksumValid("7700000001") {
		t.Error("innChecksumValid")
	}
	if !leiChecksumValid("5493001KJTIIGC8Y1R12") || leiChecksumValid("5493001KJTIIGC8Y1R13") {
		t.Error("leiChecksumValid")
	}
}

// innChecksumValid — контрольная цифра ИНН-10 юрлица (алгоритм ФНС).
func innChecksumValid(inn string) bool {
	weights := []int{2, 4, 10, 3, 5, 9, 4, 6, 8}
	if len(inn) != 10 || strings.Trim(inn, "0123456789") != "" {
		return false
	}
	sum := 0
	for i, w := range weights {
		sum += int(inn[i]-'0') * w
	}
	return sum%11%10 == int(inn[9]-'0')
}

// leiChecksumValid — ISO 17442: буквы → 10…35, всё число mod 97 = 1.
func leiChecksumValid(lei string) bool {
	var digits strings.Builder
	for _, r := range lei {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			digits.WriteString(strconv.Itoa(int(r - 'A' + 10)))
		default:
			return false
		}
	}
	n, ok := new(big.Int).SetString(digits.String(), 10)
	return ok && new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

package sanctionslist_test

import (
	"os"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml/sanctionslist"
)

type entryKey struct{ externalID, kind, name string }

func keys(entries []sanctionslist.Entry) []entryKey {
	out := make([]entryKey, len(entries))
	for i, e := range entries {
		out[i] = entryKey{e.ExternalID, e.EntityKind, e.Name}
	}
	return out
}

func TestParseUK(t *testing.T) {
	f, err := os.Open("testdata/uk_sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := sanctionslist.ParseUK(f)
	if err != nil {
		t.Fatalf("ParseUK: %v", err)
	}
	want := []entryKey{
		{"TST0001", "organization", "NORDWIND MARITIME LIMITED"},
		{"TST0001", "organization", "NORDWIND MARITIME LTD"},
		{"TST0001", "organization", "NORDWIND MARITIME LIMITED"}, // строка на каждый адрес; дубли снимает загрузчик
		{"TST0002", "person", "Ivan Petrovich TESTOV"},
		{"TST0002", "person", "Ivan TESTOFF"},
		{"TST0003", "other", "SEA FICTION"},
		// TST0004 без имени пропущен
	}
	if g := keys(got); !equalKeys(g, want) {
		t.Fatalf("entries = %+v\nwant %+v", g, want)
	}
	for _, e := range got {
		if e.CountryCode != nil {
			t.Errorf("%s: CountryCode = %q, в файле названия стран, а не ISO-коды", e.ExternalID, *e.CountryCode)
		}
		if !strings.Contains(string(e.Raw), "name_type") {
			t.Errorf("%s: raw = %s", e.ExternalID, e.Raw)
		}
	}
}

func TestParseUKMissingColumns(t *testing.T) {
	csv := "Report Date: 16-Oct-2026\nLast Updated,Unique ID,Name 1,Name 2,Name 3,Name 4,Name 5,Name type,Designation Type\n"
	_, err := sanctionslist.ParseUK(strings.NewReader(csv))
	if err == nil || !strings.Contains(err.Error(), "Name 6") {
		t.Fatalf("err = %v, want ошибку с «Name 6»", err)
	}
}

func TestMock(t *testing.T) {
	entries, err := sanctionslist.Mock()
	if err != nil {
		t.Fatalf("Mock: %v", err)
	}
	if len(entries) < 8 {
		t.Fatalf("entries = %d, ждём около 10", len(entries))
	}
	ids := map[string]int{}
	kinds := map[string]bool{}
	var gama *sanctionslist.Entry
	for i, e := range entries {
		ids[e.ExternalID]++
		kinds[e.EntityKind] = true
		if e.Name == "" || len(e.Raw) == 0 {
			t.Errorf("entry %+v: пустое имя или raw", e)
		}
		if e.ExternalID == "demo-gama-001" {
			gama = &entries[i]
		}
	}
	if gama == nil || gama.Name != "GAMA GmbH" || gama.EntityKind != "organization" || gama.CountryCode == nil || *gama.CountryCode != "DE" {
		t.Fatalf("demo-gama-001 = %+v", gama)
	}
	if !kinds["person"] || !kinds["organization"] {
		t.Errorf("kinds = %v, нужны и персоны, и организации", kinds)
	}
	aliases := 0
	for _, n := range ids {
		if n > 1 {
			aliases++
		}
	}
	if aliases == 0 {
		t.Error("нет алиасов: записей с одинаковым external_id")
	}
}

func TestParseMockUnknownKind(t *testing.T) {
	_, err := sanctionslist.ParseMock(strings.NewReader(`[{"external_id":"X","entity_kind":"ship","name":"A"}]`))
	if err == nil || !strings.Contains(err.Error(), "ship") {
		t.Fatalf("err = %v", err)
	}
}

func equalKeys(a, b []entryKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

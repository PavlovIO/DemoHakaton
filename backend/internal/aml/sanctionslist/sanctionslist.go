// Package sanctionslist — разбор санкционных списков для загрузчика (cmd/loadsanctions,
// BE-13): вымышленный мок-список (mock.json, источник mock) и UK Sanctions List FCDO в CSV
// (источник uk_fcdo). Нормализацию имён и запись в БД делает загрузчик.
package sanctionslist

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Источники — значения sanctions_entries.source (docs/db/tables/sanctions_entries.md).
const (
	SourceMock   = "mock"
	SourceUKFCDO = "uk_fcdo"
)

// Entry — одно имя записи списка. Алиасы одной записи — отдельные Entry с тем же ExternalID.
type Entry struct {
	ExternalID  string
	EntityKind  string // organization | person | other
	Name        string
	CountryCode *string // ISO-код; nil — неизвестен
	Raw         json.RawMessage
}

var entityKinds = []string{"organization", "person", "other"}

//go:embed mock.json
var mockJSON []byte

// Mock — встроенный мок-список: вымышленные записи для демо и интеграционных тестов.
// demo-gama-001 совпадает с «Gamma GmbH» из сида BE-16; остальные далеки от демо-компаний.
func Mock() ([]Entry, error) {
	return ParseMock(bytes.NewReader(mockJSON))
}

type mockEntry struct {
	ExternalID  string  `json:"external_id"`
	EntityKind  string  `json:"entity_kind"`
	Name        string  `json:"name"`
	CountryCode *string `json:"country_code"`
}

// ParseMock разбирает мок-список: JSON-массив записей; raw — сам объект записи.
func ParseMock(r io.Reader) ([]Entry, error) {
	var raws []json.RawMessage
	if err := json.NewDecoder(r).Decode(&raws); err != nil {
		return nil, fmt.Errorf("parse mock list: %w", err)
	}
	entries := make([]Entry, 0, len(raws))
	for i, raw := range raws {
		var m mockEntry
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("parse mock list: entry %d: %w", i, err)
		}
		if !slices.Contains(entityKinds, m.EntityKind) {
			return nil, fmt.Errorf("parse mock list: entry %d: entity_kind %q", i, m.EntityKind)
		}
		entries = append(entries, Entry{
			ExternalID: m.ExternalID, EntityKind: m.EntityKind, Name: m.Name, CountryCode: m.CountryCode, Raw: raw,
		})
	}
	return entries, nil
}

// Колонки UK Sanctions List CSV (сверено с файлом от 16.10.2026). Колонки берутся по
// имени: порядок в файле FCDO менял.
const (
	ukColID          = "Unique ID"
	ukColLastName    = "Name 6"
	ukColNameType    = "Name type"
	ukColDesignation = "Designation Type"
	ukColRegime      = "Regime Name"
	ukFirstNames     = 5 // Name 1 … Name 5
)

// ukKinds — Designation Type → entity_kind; Ship и прочее → other.
var ukKinds = map[string]string{"Individual": "person", "Entity": "organization"}

// ParseUK разбирает UK Sanctions List (FCDO) в CSV. Строка на каждое имя и адрес, поэтому
// одинаковые имена повторяются — дубли снимает загрузчик. Над заголовком — служебная
// строка с датой отчёта; заголовок находится по колонкам Unique ID и Name 6. CountryCode
// нет: в файле названия стран, а не ISO-коды.
func ParseUK(r io.Reader) ([]Entry, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // служебная строка короче заголовка
	cols, err := findUKHeader(cr)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse uk list: %w", err)
		}
		field := func(name string) string {
			if i := cols[name]; i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		parts := make([]string, 0, ukFirstNames+1)
		for n := 1; n <= ukFirstNames; n++ {
			if v := field(fmt.Sprintf("Name %d", n)); v != "" {
				parts = append(parts, v)
			}
		}
		if v := field(ukColLastName); v != "" {
			parts = append(parts, v)
		}
		if len(parts) == 0 {
			continue
		}
		kind, ok := ukKinds[field(ukColDesignation)]
		if !ok {
			kind = "other"
		}
		raw, err := json.Marshal(map[string]string{
			"name_type": field(ukColNameType), "designation_type": field(ukColDesignation), "regime": field(ukColRegime),
		})
		if err != nil {
			return nil, fmt.Errorf("parse uk list: raw: %w", err)
		}
		entries = append(entries, Entry{ExternalID: field(ukColID), EntityKind: kind, Name: strings.Join(parts, " "), Raw: raw})
	}
}

// findUKHeader читает строки до заголовка и возвращает номера нужных колонок.
func findUKHeader(cr *csv.Reader) (map[string]int, error) {
	required := []string{ukColID, ukColLastName, ukColNameType, ukColDesignation, ukColRegime}
	for n := 1; n <= ukFirstNames; n++ {
		required = append(required, fmt.Sprintf("Name %d", n))
	}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("parse uk list: нет заголовка с колонками %q и %q", ukColID, ukColLastName)
		}
		if err != nil {
			return nil, fmt.Errorf("parse uk list: %w", err)
		}
		cols := make(map[string]int, len(rec))
		for i, name := range rec {
			cols[strings.TrimSpace(name)] = i
		}
		if _, ok := cols[ukColID]; !ok {
			continue
		}
		var missing []string
		for _, name := range required {
			if _, ok := cols[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("parse uk list: нет колонок: %s", strings.Join(missing, ", "))
		}
		return cols, nil
	}
}

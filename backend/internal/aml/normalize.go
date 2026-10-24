package aml

import (
	"maps"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// legalForms — токены организационно-правовых форм: «ООО Альфа» и «Alfa LLC» должны
// совпадать с «Alfa». Удаляются после транслитерации, поэтому список один для кириллицы
// и латиницы (ооо → ooo).
var legalForms = map[string]bool{
	"ooo": true, "oao": true, "zao": true, "pao": true, "ao": true, "nao": true, "ip": true,
	"llc": true, "ltd": true, "limited": true, "gmbh": true, "ag": true, "inc": true,
	"incorporated": true, "corp": true, "corporation": true, "co": true, "llp": true,
	"plc": true, "jsc": true, "pjsc": true, "ojsc": true, "cjsc": true, "sa": true,
	"sas": true, "sarl": true, "bv": true, "nv": true, "srl": true, "spa": true,
}

// Normalize приводит название или имя к виду для сравнения. Одна функция для загрузчика
// (sanctions_entries.name_normalized) и скрининга: поменял её — перезагрузи списки
// (make load-sanctions), иначе совпадения пропадут молча.
func Normalize(name string) string {
	return normalizeWith(name, icao)
}

// NormalizeVariants — варианты запроса для скрининга: [ICAO] для латиницы и [ICAO,
// BGN/PCGN] для кириллицы, без дублей и пустых. Санкционные списки пишут «Yuri», а ICAO
// даёт «iurii»: JW(iurii, yuri) ≈ 0.78 — ниже порога, совпадение пропало бы (DECISIONS
// п. 41). В БД name_normalized — по ICAO (Normalize); списки в основном латинские, на них
// схема не влияет.
func NormalizeVariants(name string) []string {
	var variants []string
	for _, scheme := range []translitScheme{icao, bgnPcgn} {
		v := normalizeWith(name, scheme)
		if v != "" && !slices.Contains(variants, v) {
			variants = append(variants, v)
		}
	}
	return variants
}

// translitScheme — схема транслитерации кириллицы.
type translitScheme struct {
	letters map[rune]string
	// iotatedE — «е»/«ё» в начале слова и после гласной, «й», «ъ», «ь» пишутся «ye» (BGN/PCGN).
	iotatedE bool
}

// icao — транслитерация как в загранпаспорте (ICAO Doc 9303); «ь» выбрасывается.
var icao = translitScheme{letters: map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh",
	'з': "z", 'и': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o",
	'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts",
	'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "ie", 'ы': "y", 'ь': "", 'э': "e", 'ю': "iu",
	'я': "ia",
}}

// bgnPcgn отличается от ICAO буквами й, ю, я, ы, ъ и йотированным «е» — так имена пишут
// в санкционных списках (Yuriy, Yelena).
var bgnPcgn = translitScheme{letters: withLetters(icao.letters, map[rune]string{
	'й': "y", 'ю': "yu", 'я': "ya", 'ы': "y", 'ъ': "",
}), iotatedE: true}

func withLetters(base, override map[rune]string) map[rune]string {
	out := maps.Clone(base)
	maps.Copy(out, override)
	return out
}

// iotatingPrev — после этих букв «е» читается как «йе».
const iotatingPrev = "аеёиоуыэюяйъь"

// normalizeWith — шаги нормализации со схемой транслитерации scheme. Транслитерация идёт
// до снятия диакритики: NFKD разложил бы «й» на «и» и знак, а BGN/PCGN их различает.
func normalizeWith(name string, scheme translitScheme) string {
	latin := transliterate(norm.NFC.String(strings.ToLower(name)), scheme)
	stripped, _, err := transform.String(stripDiacritics(), latin)
	if err != nil {
		stripped = latin // transform.String на валидном UTF-8 не ошибается
	}
	var b strings.Builder
	for _, r := range stripped {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteRune(' ')
	}
	tokens := strings.Fields(b.String())
	kept := tokens[:0]
	for _, t := range tokens {
		if !legalForms[t] {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, " ")
}

// transliterate заменяет кириллицу по таблице; остальные руны — как есть.
func transliterate(s string, scheme translitScheme) string {
	var b strings.Builder
	prev := ' '
	for _, r := range s {
		switch latin, ok := scheme.letters[r]; {
		case scheme.iotatedE && (r == 'е' || r == 'ё') && (!unicode.IsLetter(prev) || strings.ContainsRune(iotatingPrev, prev)):
			b.WriteString("ye")
		case ok:
			b.WriteString(latin)
		default:
			b.WriteRune(r)
		}
		prev = r
	}
	return b.String()
}

// sortTokens упорядочивает слова нормализованной строки: «petrov ivan» → «ivan petrov».
func sortTokens(s string) string {
	tokens := strings.Fields(s)
	slices.Sort(tokens)
	return strings.Join(tokens, " ")
}

// stripDiacritics снимает диакритику: NFKD раскладывает «ü» на «u» и знак, знак удаляется.
func stripDiacritics() transform.Transformer {
	return transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
}

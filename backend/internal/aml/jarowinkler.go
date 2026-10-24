package aml

// Параметры Winkler: бонус за общий префикс до 4 символов с весом 0.1 — стандартные
// значения, на них откалиброваны пороги скрининга 0.85 и 0.90.
const (
	winklerPrefixScale = 0.1
	winklerMaxPrefix   = 4
)

// jaroWinkler — сходство строк по рунам, от 0 до 1. Свой вариант вместо зависимости:
// функция одна, а эталоны в тестах фиксируют реализацию.
func jaroWinkler(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	jaro := jaroSimilarity(ra, rb)
	prefix := 0
	for prefix < min(len(ra), len(rb), winklerMaxPrefix) && ra[prefix] == rb[prefix] {
		prefix++
	}
	return jaro + float64(prefix)*winklerPrefixScale*(1-jaro)
}

func jaroSimilarity(a, b []rune) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Символы совпадают, если равны и стоят не дальше window друг от друга.
	window := max(len(a), len(b))/2 - 1
	window = max(window, 0)
	matchedA := make([]bool, len(a))
	matchedB := make([]bool, len(b))
	matches := 0
	for i := range a {
		for j := max(0, i-window); j < min(len(b), i+window+1); j++ {
			if matchedB[j] || a[i] != b[j] {
				continue
			}
			matchedA[i], matchedB[j] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}
	// Транспозиции — совпавшие символы, стоящие в другом порядке; считаются парами.
	transpositions, j := 0, 0
	for i := range a {
		if !matchedA[i] {
			continue
		}
		for !matchedB[j] {
			j++
		}
		if a[i] != b[j] {
			transpositions++
		}
		j++
	}
	m := float64(matches)
	return (m/float64(len(a)) + m/float64(len(b)) + (m-float64(transpositions)/2)/m) / 3
}

// nameScore — финальный score совпадения имён. JW чувствителен к порядку слов, а в списках
// встречаются и «Petrov Ivan», и «Ivan Petrov» — поэтому берётся максимум с JW
// отсортированных токенов.
func nameScore(a, b string) float64 {
	return max(jaroWinkler(a, b), jaroWinkler(sortTokens(a), sortTokens(b)))
}

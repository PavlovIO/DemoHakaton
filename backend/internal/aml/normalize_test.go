package aml

import (
	"slices"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  Gamma   Holding  ", "gamma holding"},
		{"IVANOV IVAN", "ivanov ivan"},
		{"Müller GmbH", "muller"},
		{"Beta Corp Ltd.", "beta"},
		{"ООО «Альфа»", "alfa"},
		{"Альфа, ООО", "alfa"},
		{"Щукин Юрий", "shchukin iurii"},
		{"Иванов Иван", "ivanov ivan"},
		{"АО", ""},
		{"Хрущёв Объединённый Цех Ярэ", "khrushchev obieedinennyi tsekh iare"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeVariants(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Юрий Щукин", []string{"iurii shchukin", "yuriy shchukin"}},
		{"Yuri Shchukin", []string{"yuri shchukin"}},
		{"Иванов Иван", []string{"ivanov ivan"}}, // варианты совпали — без дублей
		{"Елена", []string{"elena", "yelena"}},
		{"Объявление Майя", []string{"obieiavlenie maiia", "obyavleniye mayya"}},
		{"АО", nil},
	}
	for _, c := range cases {
		if got := NormalizeVariants(c.in); !slices.Equal(got, c.want) {
			t.Errorf("NormalizeVariants(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

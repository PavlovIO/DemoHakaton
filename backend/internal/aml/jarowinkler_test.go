package aml

import (
	"math"
	"testing"
)

const scoreTolerance = 0.001

// Эталоны — из статьи Winkler (1990) и стандартных примеров Jaro-Winkler.
func TestJaroWinkler(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"martha", "marhta", 0.961},
		{"dwayne", "duane", 0.840},
		{"dixon", "dicksonx", 0.813},
		{"abc", "abc", 1},
		{"abc", "xyz", 0},
		{"", "", 1},
		{"abc", "", 0},
		{"юрий", "юрий", 1},
	}
	for _, c := range cases {
		if got := jaroWinkler(c.a, c.b); math.Abs(got-c.want) > scoreTolerance {
			t.Errorf("jaroWinkler(%q, %q) = %.4f, want %.3f", c.a, c.b, got, c.want)
		}
	}
}

func TestNameScore(t *testing.T) {
	if got := nameScore("petrov ivan", "ivan petrov"); got != 1 {
		t.Errorf("nameScore(petrov ivan, ivan petrov) = %.4f, want 1", got)
	}
}

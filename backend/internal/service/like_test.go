package service

import "testing"

func TestEscapeLike(t *testing.T) {
	cases := []struct{ in, want string }{
		{`a_b%c\`, `a\_b\%c\\`},
		{"ООО Альфа", "ООО Альфа"},
		{"", ""},
	}
	for _, c := range cases {
		if got := EscapeLike(c.in); got != c.want {
			t.Errorf("EscapeLike(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

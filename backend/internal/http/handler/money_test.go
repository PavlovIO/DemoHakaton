package handler_test

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"

	_ "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/handler" // init: TrimTrailingZeros
)

// Контракт обещает суммы строками с масштабом из БД ("500000.00", "8.50"), а не "500000"/"8.5":
// shopspring/decimal по умолчанию срезает хвостовые нули при сериализации.
func TestMoneyKeepsScaleOnTheWire(t *testing.T) {
	for _, s := range []string{"500000.00", "8.50", "0.10", "1"} {
		// api.Money — алиас decimal.Decimal: сериализуется тем же MarshalJSON.
		got, err := json.Marshal(decimal.RequireFromString(s))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != `"`+s+`"` {
			t.Errorf("Money(%s) -> %s", s, got)
		}
	}
}

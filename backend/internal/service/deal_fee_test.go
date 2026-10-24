package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestDealFee(t *testing.T) {
	tests := []struct {
		name       string
		amount     string
		feePercent *string // nil — комиссии нет
		want       *string // nil — fee_amount NULL
	}{
		{"кейс issue: 1 % от 500 000", "500000.00", ptr("1"), ptr("5000.00")},
		{"дробный процент", "500000.00", ptr("1.2500"), ptr("6250.00")},
		{"округление вниз до копеек", "1234.56", ptr("0.3333"), ptr("4.11")},
		{"ровно половина копейки — от нуля", "0.50", ptr("1"), ptr("0.01")},
		{"нулевая комиссия", "100.00", ptr("0"), ptr("0.00")},
		{"без комиссии — NULL", "100.00", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee := decimal.NullDecimal{}
			if tt.feePercent != nil {
				fee = decimal.NewNullDecimal(decimal.RequireFromString(*tt.feePercent))
			}
			got := dealFee(decimal.RequireFromString(tt.amount), fee)
			if tt.want == nil {
				if got.Valid {
					t.Fatalf("fee_amount = %s, want NULL", got.Decimal)
				}
				return
			}
			want := decimal.RequireFromString(*tt.want)
			if !got.Valid || !got.Decimal.Equal(want) || got.Decimal.StringFixed(2) != *tt.want {
				t.Fatalf("fee_amount = %v, want %s", got, *tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

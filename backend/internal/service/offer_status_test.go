package service

import (
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

func TestNextOfferStatus(t *testing.T) {
	const denied = OfferStatus("") // 409 INVALID_STATUS_TRANSITION
	// Ожидания — из «Таблицы переходов оффера» плана BE-08: из sent разрешены 3 действия,
	// accepted, rejected и withdrawn — конечные.
	tests := []struct {
		from   OfferStatus
		action OfferAction
		want   OfferStatus
	}{
		{OfferStatusSent, OfferAccept, OfferStatusAccepted},
		{OfferStatusSent, OfferReject, OfferStatusRejected},
		{OfferStatusSent, OfferWithdraw, OfferStatusWithdrawn},

		{OfferStatusAccepted, OfferAccept, denied},
		{OfferStatusAccepted, OfferReject, denied},
		{OfferStatusAccepted, OfferWithdraw, denied},

		{OfferStatusRejected, OfferAccept, denied},
		{OfferStatusRejected, OfferReject, denied},
		{OfferStatusRejected, OfferWithdraw, denied},

		{OfferStatusWithdrawn, OfferAccept, denied},
		{OfferStatusWithdrawn, OfferReject, denied},
		{OfferStatusWithdrawn, OfferWithdraw, denied},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"+"+string(tt.action), func(t *testing.T) {
			got, err := NextOfferStatus(tt.from, tt.action)
			if tt.want == denied {
				if apperr.CodeOf(err) != apperr.CodeInvalidTransition {
					t.Fatalf("err = %v, want INVALID_STATUS_TRANSITION", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

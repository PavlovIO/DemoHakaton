package kyc

import (
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

func TestNextKycStatus(t *testing.T) {
	const (
		draft       = repository.KycStatusDraft
		underReview = repository.KycStatusUnderReview
		verified    = repository.KycStatusVerified
		rejected    = repository.KycStatusRejected
		denied      = repository.KycStatus("") // 409 INVALID_STATUS_TRANSITION
	)
	tests := []struct {
		from   repository.KycStatus
		action KycAction
		want   repository.KycStatus
	}{
		{draft, KycEdit, draft},
		{draft, KycSubmit, underReview},
		{draft, KycApprove, denied},
		{draft, KycReject, denied},

		{underReview, KycEdit, denied},
		{underReview, KycSubmit, denied},
		{underReview, KycApprove, verified},
		{underReview, KycReject, rejected},

		{verified, KycEdit, denied},
		{verified, KycSubmit, denied},
		{verified, KycApprove, denied},
		{verified, KycReject, denied},

		{rejected, KycEdit, rejected},
		{rejected, KycSubmit, underReview},
		{rejected, KycApprove, denied},
		{rejected, KycReject, denied},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"+"+string(tt.action), func(t *testing.T) {
			got, err := NextKycStatus(tt.from, tt.action)
			if tt.want == denied {
				e, ok := apperr.As(err)
				if !ok || e.Code != apperr.CodeInvalidTransition {
					t.Fatalf("err = %v, want INVALID_STATUS_TRANSITION", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

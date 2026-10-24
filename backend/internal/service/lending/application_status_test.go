package lending

import (
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

func TestNextApplicationStatus(t *testing.T) {
	const denied = ApplicationStatus("") // 409 INVALID_STATUS_TRANSITION
	// Ожидания — из «Таблицы переходов заявки» плана BE-07: 9 разрешённых пар, 21 запрещённая.
	tests := []struct {
		from   ApplicationStatus
		action ApplicationAction
		want   ApplicationStatus
	}{
		{AppStatusDraft, AppSubmit, AppStatusModeration},
		{AppStatusDraft, AppApprove, denied},
		{AppStatusDraft, AppReject, denied},
		{AppStatusDraft, AppCancel, AppStatusCancelled},
		{AppStatusDraft, AppAgree, denied},

		{AppStatusModeration, AppSubmit, denied},
		{AppStatusModeration, AppApprove, AppStatusPublished},
		{AppStatusModeration, AppReject, AppStatusRejected},
		{AppStatusModeration, AppCancel, AppStatusCancelled},
		{AppStatusModeration, AppAgree, denied},

		{AppStatusRejected, AppSubmit, AppStatusModeration},
		{AppStatusRejected, AppApprove, denied},
		{AppStatusRejected, AppReject, denied},
		{AppStatusRejected, AppCancel, AppStatusCancelled},
		{AppStatusRejected, AppAgree, denied},

		{AppStatusPublished, AppSubmit, denied},
		{AppStatusPublished, AppApprove, denied},
		{AppStatusPublished, AppReject, denied},
		{AppStatusPublished, AppCancel, AppStatusCancelled},
		{AppStatusPublished, AppAgree, AppStatusAgreed},

		{AppStatusAgreed, AppSubmit, denied},
		{AppStatusAgreed, AppApprove, denied},
		{AppStatusAgreed, AppReject, denied},
		{AppStatusAgreed, AppCancel, denied},
		{AppStatusAgreed, AppAgree, denied},

		{AppStatusCancelled, AppSubmit, denied},
		{AppStatusCancelled, AppApprove, denied},
		{AppStatusCancelled, AppReject, denied},
		{AppStatusCancelled, AppCancel, denied},
		{AppStatusCancelled, AppAgree, denied},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"+"+string(tt.action), func(t *testing.T) {
			got, err := NextApplicationStatus(tt.from, tt.action)
			if tt.want == denied {
				if apperr.CodeOf(err) != apperr.CodeInvalidTransition {
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

func TestCanEditApplication(t *testing.T) {
	tests := map[ApplicationStatus]bool{
		AppStatusDraft: true, AppStatusModeration: false, AppStatusRejected: true,
		AppStatusPublished: false, AppStatusAgreed: false, AppStatusCancelled: false,
	}
	for status, want := range tests {
		if got := canEditApplication(status); got != want {
			t.Errorf("canEditApplication(%s) = %v, want %v", status, got, want)
		}
	}
}

func TestCanRejectAllOffers(t *testing.T) {
	tests := map[ApplicationStatus]bool{
		AppStatusDraft: false, AppStatusModeration: false, AppStatusRejected: false,
		AppStatusPublished: true, AppStatusAgreed: false, AppStatusCancelled: false,
	}
	for status, want := range tests {
		if got := canRejectAllOffers(status); got != want {
			t.Errorf("canRejectAllOffers(%s) = %v, want %v", status, got, want)
		}
	}
}

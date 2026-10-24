package lending

import (
	"slices"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

// ApplicationStatus — статус заявки (enum application_status).
type ApplicationStatus string

// Статусы заявки. agreed и cancelled — конечные.
const (
	AppStatusDraft      ApplicationStatus = "draft"
	AppStatusModeration ApplicationStatus = "moderation"
	AppStatusRejected   ApplicationStatus = "rejected"
	AppStatusPublished  ApplicationStatus = "published"
	AppStatusAgreed     ApplicationStatus = "agreed"
	AppStatusCancelled  ApplicationStatus = "cancelled"
)

// ApplicationAction — действие, меняющее статус заявки.
type ApplicationAction string

// Действия над заявкой: submit и cancel — заёмщик, approve и reject — админ (BE-12),
// agree — принятие оффера (BE-09).
const (
	AppSubmit  ApplicationAction = "submit"
	AppApprove ApplicationAction = "approve"
	AppReject  ApplicationAction = "reject"
	AppCancel  ApplicationAction = "cancel"
	AppAgree   ApplicationAction = "agree"
)

// applicationTransitions — единственная таблица переходов заявки (ADR-0001, «Статусные
// машины»). У действия ровно один целевой статус; текущий должен входить в from, иначе 409.
var applicationTransitions = map[ApplicationAction]struct {
	from []ApplicationStatus
	to   ApplicationStatus
}{
	AppSubmit:  {from: []ApplicationStatus{AppStatusDraft, AppStatusRejected}, to: AppStatusModeration},
	AppApprove: {from: []ApplicationStatus{AppStatusModeration}, to: AppStatusPublished},
	AppReject:  {from: []ApplicationStatus{AppStatusModeration}, to: AppStatusRejected},
	// Отменить можно всё, что не стало сделкой и ещё не отменено.
	AppCancel: {from: []ApplicationStatus{AppStatusDraft, AppStatusModeration, AppStatusRejected, AppStatusPublished}, to: AppStatusCancelled},
	AppAgree:  {from: []ApplicationStatus{AppStatusPublished}, to: AppStatusAgreed},
}

// NextApplicationStatus — статус после действия или 409 INVALID_STATUS_TRANSITION.
func NextApplicationStatus(from ApplicationStatus, action ApplicationAction) (ApplicationStatus, error) {
	t, ok := applicationTransitions[action]
	if !ok || !slices.Contains(t.from, from) {
		return "", apperr.InvalidTransition(string(from), string(t.to))
	}
	return t.to, nil
}

// canEditApplication — PATCH заявки разрешён, пока её не видят модератор и кредиторы:
// иначе админ одобрил бы или кредитор оценил бы не то, что видел.
func canEditApplication(s ApplicationStatus) bool {
	return s == AppStatusDraft || s == AppStatusRejected
}

// canRejectAllOffers — отклонить все офферы можно только у опубликованной заявки:
// офферы в статусе sent бывают только у неё.
func canRejectAllOffers(s ApplicationStatus) bool {
	return s == AppStatusPublished
}

package kyc

import (
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// KycAction — действие над KYC компании.
type KycAction string

// Действия над KYC: правка профиля и документов, отправка на проверку, решения админа.
const (
	KycEdit    KycAction = "edit"    // PATCH профиля, загрузка и удаление документа
	KycSubmit  KycAction = "submit"  // submitKyc
	KycApprove KycAction = "approve" // решение админа (BE-12)
	KycReject  KycAction = "reject"  // решение админа (BE-12)
)

// kycTransitions — единственная таблица переходов KYC (ADR-0001, «Статусные машины»).
// Отсутствие пары «статус × действие» = 409. edit не меняет статус: правка разрешена
// только пока компания не на проверке и не верифицирована — иначе админ одобрил бы
// не то, что видел.
var kycTransitions = map[repository.KycStatus]map[KycAction]repository.KycStatus{
	repository.KycStatusDraft: {
		KycEdit:   repository.KycStatusDraft,
		KycSubmit: repository.KycStatusUnderReview,
	},
	repository.KycStatusUnderReview: {
		KycApprove: repository.KycStatusVerified,
		KycReject:  repository.KycStatusRejected,
	},
	repository.KycStatusRejected: {
		KycEdit:   repository.KycStatusRejected,
		KycSubmit: repository.KycStatusUnderReview,
	},
	// verified — конечный: исходящих переходов нет.
}

// kycActionTarget — в какой статус ведёт действие; только для текста ошибки 409.
var kycActionTarget = map[KycAction]repository.KycStatus{
	KycSubmit:  repository.KycStatusUnderReview,
	KycApprove: repository.KycStatusVerified,
	KycReject:  repository.KycStatusRejected,
}

// kycEditDenied — 409 на правку профиля или документов вне draft и rejected.
func kycEditDenied() *apperr.Error {
	return apperr.New(apperr.CodeInvalidTransition, "Профиль и документы можно менять только в статусах draft и rejected")
}

// NextKycStatus — статус после действия или 409 INVALID_STATUS_TRANSITION. Экспортирована
// для решений админа (service/admin): таблица переходов остаётся одна (ADR-0001).
func NextKycStatus(from repository.KycStatus, action KycAction) (repository.KycStatus, error) {
	if to, ok := kycTransitions[from][action]; ok {
		return to, nil
	}
	if action == KycEdit {
		return "", kycEditDenied()
	}
	return "", apperr.InvalidTransition(string(from), string(kycActionTarget[action]))
}

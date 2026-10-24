package service

import (
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

// OfferStatus — статус оффера (enum offer_status).
type OfferStatus string

// Статусы оффера. accepted, rejected и withdrawn — конечные.
const (
	OfferStatusSent      OfferStatus = "sent"
	OfferStatusAccepted  OfferStatus = "accepted"
	OfferStatusRejected  OfferStatus = "rejected"
	OfferStatusWithdrawn OfferStatus = "withdrawn"
)

// OfferAction — действие, меняющее статус оффера.
type OfferAction string

// Действия над оффером: accept (BE-09) и reject — заёмщик-владелец заявки, withdraw —
// кредитор-автор. Системные и массовые отклонения (BE-07, BE-09) — тот же reject.
const (
	OfferAccept   OfferAction = "accept"
	OfferReject   OfferAction = "reject"
	OfferWithdraw OfferAction = "withdraw"
)

// offerTransitions — единственная таблица переходов оффера (ADR-0001, «Статусные машины»).
// Все действия разрешены только из sent: решение по офферу принимается один раз.
var offerTransitions = map[OfferAction]struct {
	from OfferStatus
	to   OfferStatus
}{
	OfferAccept:   {from: OfferStatusSent, to: OfferStatusAccepted},
	OfferReject:   {from: OfferStatusSent, to: OfferStatusRejected},
	OfferWithdraw: {from: OfferStatusSent, to: OfferStatusWithdrawn},
}

// NextOfferStatus — статус после действия или 409 INVALID_STATUS_TRANSITION.
func NextOfferStatus(from OfferStatus, action OfferAction) (OfferStatus, error) {
	t, ok := offerTransitions[action]
	if !ok || t.from != from {
		return "", apperr.InvalidTransition(string(from), string(t.to))
	}
	return t.to, nil
}

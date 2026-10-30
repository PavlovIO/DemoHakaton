import { useState } from 'react'
import { toast } from 'sonner'
import { Trans, useTranslation } from 'react-i18next'
import { useAuth } from '@/hooks/useAuth'
import {
  useAcceptOffer,
  useMyOffers,
  useRejectOffer,
  useWithdrawOffer,
  type Offer,
} from '@/hooks/useOffers'
import { handleApiError } from '@/lib/api-errors'
import { formatMoney, formatPercent } from '@/lib/money'
import { offerStatusLabel, repaymentScheduleLabel } from '@/lib/labels'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { ChatModal } from '@/components/ChatModal'
import { LoadMore } from '@/components/ui/LoadMore'

// Вкладка «Предложения»: GET /offers role-scoped — кредитору отдаёт офферы его компании,
// заёмщику — все входящие предложения на заявки его компании (все статусы, как архив).
export function Offers() {
  const { user } = useAuth()
  return user?.role === 'borrower' ? <IncomingOffers /> : <MyOutgoingOffers />
}

function getStatusBadge(status: Offer['status']) {
  const label = offerStatusLabel(status)
  switch (status) {
    case 'sent': return <Badge variant="warning">{label}</Badge>
    case 'accepted': return <Badge variant="success">{label}</Badge>
    case 'rejected': return <Badge variant="danger">{label}</Badge>
    case 'withdrawn': return <Badge variant="default">{label}</Badge>
  }
}

function OfferRow({ offer, children }: { offer: Offer; children?: React.ReactNode }) {
  const { t } = useTranslation()
  return (
    <li className="p-4 hover:bg-gray-50">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="flex-1">
          <div className="flex items-center gap-3 mb-1">
            <span className="font-mono text-sm text-gray-500">{offer.application_number}</span>
            {getStatusBadge(offer.status)}
          </div>
          <p className="text-sm text-gray-900">
            <Trans
              i18nKey="offers.proposed"
              values={{ amount: formatMoney(offer.amount, offer.currency), rate: formatPercent(offer.interest_rate_annual) }}
              components={{ b: <span className="font-medium" /> }}
            />
          </p>
          <p className="text-xs text-gray-500 mt-1">
            {t('offers.details', {
              fee: formatPercent(offer.fee_percent),
              schedule: repaymentScheduleLabel(offer.repayment_schedule),
            })}
          </p>
          {/* Заёмщику нужен кредитор-автор; кредитору своя строка не нужна — он сам автор. */}
          {offer.lender?.name && (
            <p className="text-xs text-gray-500 mt-1">{t('offers.lender', { name: offer.lender.name })}</p>
          )}
          {offer.status === 'rejected' && offer.reject_comment && (
            <p className="text-xs text-danger mt-1">{t('offers.rejectComment', { comment: offer.reject_comment })}</p>
          )}
        </div>
        <div className="flex gap-2">{children}</div>
      </div>
    </li>
  )
}

// Кредитор: отправленные им предложения; из sent можно отозвать (accept/reject — право заёмщика).
function MyOutgoingOffers() {
  const { t } = useTranslation()
  const offersQuery = useMyOffers()
  const { data, isLoading } = offersQuery
  const offers = data?.items ?? []
  const withdrawOffer = useWithdrawOffer()
  const [chatOffer, setChatOffer] = useState<Offer | null>(null)

  const handleWithdraw = (id: string) => {
    if (confirm(t('offers.withdrawConfirm'))) {
      withdrawOffer.mutate(id, {
        onSuccess: () => toast.success(t('offers.withdrawn')),
        onError: handleApiError,
      })
    }
  }

  return (
    <div className="space-y-6 relative">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold text-gray-900">{t('offers.title')}</h1>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {offers.map((offer) => (
            <OfferRow key={offer.id} offer={offer}>
              <Button variant="outline" onClick={() => setChatOffer(offer)}>{t('offers.chat')}</Button>
              {offer.status === 'sent' && (
                <Button variant="danger" disabled={withdrawOffer.isPending} onClick={() => handleWithdraw(offer.id)}>
                  {t('offers.withdraw')}
                </Button>
              )}
            </OfferRow>
          ))}
          {!isLoading && offers.length === 0 && (
            <li className="p-8 text-center text-gray-500">
              {t('offers.empty')}
            </li>
          )}
        </ul>
        <LoadMore query={offersQuery} />
      </div>

      <ChatModal
        key={chatOffer?.id}
        isOpen={!!chatOffer}
        onClose={() => setChatOffer(null)}
        offerId={chatOffer?.id || ''}
        offerNumber={chatOffer?.application_number || ''}
      />
    </div>
  )
}

// Заёмщик: все входящие предложения по всем заявкам компании. Принять или отклонить можно
// только sent-оффер опубликованной заявки — это проверяет бэкенд (409/403), здесь те же условия в UI.
function IncomingOffers() {
  const { t } = useTranslation()
  const offersQuery = useMyOffers()
  const { data, isLoading } = offersQuery
  const offers = data?.items ?? []
  const acceptOffer = useAcceptOffer()
  const rejectOffer = useRejectOffer()
  const [chatOffer, setChatOffer] = useState<Offer | null>(null)

  const handleAccept = (offer: Offer) => {
    if (!confirm(t('offers.acceptConfirm'))) return
    acceptOffer.mutate(offer.id, {
      // Принятие одного оффера фиксирует сделку и отклоняет остальные sent-предложения заявки.
      onSuccess: (deal) => toast.success(t('offers.accepted', { number: deal.number })),
      onError: handleApiError,
    })
  }

  const handleReject = (offer: Offer) => {
    const raw = prompt(t('offers.rejectPrompt'))
    // null — «Отмена» диалога, действия нет; пустая строка — отклонение без комментария.
    if (raw === null) return
    rejectOffer.mutate(
      { offerId: offer.id, comment: raw.trim() || undefined },
      {
        onSuccess: () => toast.success(t('offers.rejectedToast')),
        onError: handleApiError,
      },
    )
  }

  return (
    <div className="space-y-6 relative">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold text-gray-900">{t('offers.incomingTitle')}</h1>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {offers.map((offer) => (
            <OfferRow key={offer.id} offer={offer}>
              <Button variant="outline" onClick={() => setChatOffer(offer)}>{t('offers.chat')}</Button>
              {offer.status === 'sent' && (
                <>
                  <Button variant="outline" disabled={rejectOffer.isPending} onClick={() => handleReject(offer)}>
                    {t('offers.reject')}
                  </Button>
                  <Button disabled={acceptOffer.isPending} onClick={() => handleAccept(offer)}>
                    {t('offers.accept')}
                  </Button>
                </>
              )}
            </OfferRow>
          ))}
          {!isLoading && offers.length === 0 && (
            <li className="p-8 text-center text-gray-500">
              {t('offers.incomingEmpty')}
            </li>
          )}
        </ul>
        <LoadMore query={offersQuery} />
      </div>

      <ChatModal
        key={chatOffer?.id}
        isOpen={!!chatOffer}
        onClose={() => setChatOffer(null)}
        offerId={chatOffer?.id || ''}
        offerNumber={chatOffer?.application_number || ''}
        counterpartyName={chatOffer?.lender?.name}
      />
    </div>
  )
}

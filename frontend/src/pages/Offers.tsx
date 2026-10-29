import { useState } from 'react'
import { toast } from 'sonner'
import { Trans, useTranslation } from 'react-i18next'
import { useAuth } from '@/hooks/useAuth'
import { useMyOffers, useWithdrawOffer, type Offer } from '@/hooks/useOffers'
import { handleApiError } from '@/lib/api-errors'
import { formatMoney, formatPercent } from '@/lib/money'
import { offerStatusLabel, repaymentScheduleLabel } from '@/lib/labels'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { ChatModal } from '@/components/ChatModal'
import { LoadMore } from '@/components/ui/LoadMore'

export function Offers() {
  const { user } = useAuth()

  // Проверка роли до запросов: заёмщику GET /offers не нужен, у него входящие в карточке заявки.
  if (user?.role === 'borrower') {
    return <div className="p-6">Входящие предложения отображаются в карточке заявки.</div>
  }
  return <MyOffers />
}

function MyOffers() {
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

  const getStatusBadge = (status: Offer['status']) => {
    const label = offerStatusLabel(status)
    switch (status) {
      case 'sent': return <Badge variant="warning">{label}</Badge>
      case 'accepted': return <Badge variant="success">{label}</Badge>
      case 'rejected': return <Badge variant="danger">{label}</Badge>
      case 'withdrawn': return <Badge variant="default">{label}</Badge>
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
            <li key={offer.id} className="p-4 hover:bg-gray-50">
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
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" onClick={() => setChatOffer(offer)}>{t('offers.chat')}</Button>
                  {offer.status === 'sent' && (
                    <Button variant="danger" disabled={withdrawOffer.isPending} onClick={() => handleWithdraw(offer.id)}>{t('offers.withdraw')}</Button>
                  )}
                </div>
              </div>
            </li>
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
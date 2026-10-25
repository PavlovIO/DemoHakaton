import { useState } from 'react'
import { toast } from 'sonner'
import { useAuth } from '@/hooks/useAuth'
import { useMyOffers, useWithdrawOffer, type Offer } from '@/hooks/useOffers'
import { handleApiError } from '@/lib/api-errors'
import { formatMoney, formatPercent } from '@/lib/money'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { ChatModal } from '@/components/ChatModal'

export function Offers() {
  const { user } = useAuth()
  const { data } = useMyOffers()
  const offers = data?.items ?? []
  const withdrawOffer = useWithdrawOffer()
  const [chatOffer, setChatOffer] = useState<Offer | null>(null)

  const handleWithdraw = (id: string) => {
    if (confirm('Вы уверены, что хотите отозвать предложение?')) {
      withdrawOffer.mutate(id, {
        onSuccess: () => toast.success('Предложение отозвано'),
        onError: handleApiError,
      })
    }
  }

  const getStatusBadge = (status: Offer['status']) => {
    switch (status) {
      case 'sent': return <Badge variant="warning">Отправлен</Badge>
      case 'accepted': return <Badge variant="success">Принят</Badge>
      case 'rejected': return <Badge variant="danger">Отклонен</Badge>
      case 'withdrawn': return <Badge variant="default">Отозван</Badge>
    }
  }

  // Заемщик видит заглушку (для него эта страница не нужна, у него будут входящие)
  if (user?.role === 'borrower') {
    return <div className="p-6">Входящие предложения отображаются в карточке заявки.</div>
  }

  return (
    <div className="space-y-6 relative">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold text-gray-900">Мои предложения</h1>
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
                    Вы предложили: <span className="font-medium">{formatMoney(offer.amount, offer.currency)}</span> под <span className="font-medium">{formatPercent(offer.interest_rate_annual)}</span> годовых
                  </p>
                  <p className="text-xs text-gray-500 mt-1">
                    Комиссия: {formatPercent(offer.fee_percent)} | График: {offer.repayment_schedule === 'bullet' ? 'В конце срока' : offer.repayment_schedule}
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" onClick={() => setChatOffer(offer)}>Чат</Button>
                  {offer.status === 'sent' && (
                    <Button variant="danger" disabled={withdrawOffer.isPending} onClick={() => handleWithdraw(offer.id)}>Отозвать</Button>
                  )}
                </div>
              </div>
            </li>
          ))}
          {offers.length === 0 && (
            <li className="p-8 text-center text-gray-500">
              Вы еще не отправляли предложений. Перейдите в «Витрину заявок», чтобы найти интересные проекты.
            </li>
          )}
        </ul>
      </div>

      <ChatModal
        isOpen={!!chatOffer}
        onClose={() => setChatOffer(null)}
        offerId={chatOffer?.id || ''}
        offerNumber={chatOffer?.application_number || ''}
      />
    </div>
  )
}
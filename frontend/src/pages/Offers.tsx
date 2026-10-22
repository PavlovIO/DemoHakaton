import { useState, useEffect } from 'react'
import { useAuth } from '@/hooks/useAuth'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'

interface Offer {
  id: string
  application_id: string
  application_number: string
  company_name: string
  amount: string
  currency: string
  interest_rate_annual: string
  fee_percent: string
  repayment_schedule: string
  status: 'sent' | 'accepted' | 'rejected' | 'withdrawn'
  created_at: string
}

export function Offers() {
  const { user } = useAuth()
  const [offers, setOffers] = useState<Offer[]>([])

  // Загружаем офферы из localStorage при открытии страницы
  useEffect(() => {
    const saved = JSON.parse(localStorage.getItem('mock_offers') || '[]')
    setOffers(saved)
  }, [])

  const handleWithdraw = (id: string) => {
    if (confirm('Вы уверены, что хотите отозвать предложение?')) {
      const updated = offers.map(o => o.id === id ? { ...o, status: 'withdrawn' } : o)
      setOffers(updated)
      localStorage.setItem('mock_offers', JSON.stringify(updated))
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
    <div className="space-y-6">
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
                    <span className="text-sm font-semibold text-gray-900">{offer.company_name}</span>
                  </div>
                  <p className="text-sm text-gray-900">
                    Вы предложили: <span className="font-medium">{Number(offer.amount).toLocaleString('ru-RU')} {offer.currency}</span> под <span className="font-medium">{offer.interest_rate_annual}%</span> годовых
                  </p>
                  <p className="text-xs text-gray-500 mt-1">
                    Комиссия: {offer.fee_percent}% | График: {offer.repayment_schedule === 'bullet' ? 'В конце срока' : offer.repayment_schedule}
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" onClick={() => alert('Здесь будет чат (FE-09)')}>Чат</Button>
                  {offer.status === 'sent' && (
                    <Button variant="danger" onClick={() => handleWithdraw(offer.id)}>Отозвать</Button>
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
    </div>
  )
}
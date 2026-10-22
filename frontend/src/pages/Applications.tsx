import { useState, useEffect } from 'react'
import { useAuth } from '@/hooks/useAuth'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { addNotification } from '@/lib/notifications'
import { ChatModal } from '@/components/ChatModal'

interface Application {
  id: string
  number: string
  company_name?: string
  amount: string
  currency: string
  target_country_code: string
  purpose: string
  term_months: number
  status: 'draft' | 'moderation' | 'published' | 'agreed' | 'rejected' | 'cancelled'
  created_at: string
}

interface Offer {
  id: string
  application_id: string
  application_number?: string
  company_name: string
  amount: string
  currency: string
  interest_rate_annual: string
  fee_percent: string
  repayment_schedule: string
  status: 'sent' | 'accepted' | 'rejected' | 'withdrawn'
}

const INITIAL_APPLICATIONS: Application[] = [
  {
    id: '1',
    number: 'APP-000001',
    company_name: 'Альфа',
    amount: '500000.00',
    currency: 'EUR',
    target_country_code: 'DE',
    purpose: 'Закупка промышленного оборудования в Германии',
    term_months: 6,
    status: 'published',
    created_at: new Date().toISOString(),
  }
]

const getStatusBadge = (status: Application['status']) => {
  switch (status) {
    case 'draft': return <Badge variant="default">Черновик</Badge>
    case 'moderation': return <Badge variant="warning">На модерации</Badge>
    case 'published': return <Badge variant="success">Опубликована</Badge>
    case 'agreed': return <Badge variant="primary">Согласована</Badge>
    case 'rejected': return <Badge variant="danger">Отклонена</Badge>
    case 'cancelled': return <Badge variant="default">Отменена</Badge>
    default: return <Badge>{status}</Badge>
  }
}

export function Applications() {
  const { user } = useAuth()
  const [applications, setApplications] = useState<Application[]>([])
  const [offers, setOffers] = useState<Offer[]>([])
  
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [selectedAppForOffers, setSelectedAppForOffers] = useState<Application | null>(null)
  
  const [chatOffer, setChatOffer] = useState<Offer | null>(null) // <-- ДОБАВЛЕН СТЕЙТ ДЛЯ ЧАТА
  
  const [formData, setFormData] = useState({ amount: '', currency: 'EUR', target_country_code: 'DE', purpose: '', term_months: '6' })

  useEffect(() => {
    const savedApps = JSON.parse(localStorage.getItem('mock_applications') || '[]')
    if (savedApps.length === 0) {
      localStorage.setItem('mock_applications', JSON.stringify(INITIAL_APPLICATIONS))
      setApplications(INITIAL_APPLICATIONS)
    } else {
      setApplications(savedApps)
    }

    const savedOffers = JSON.parse(localStorage.getItem('mock_offers') || '[]')
    setOffers(savedOffers)
  }, [])

  if (user?.role !== 'borrower') {
    return <div className="p-6">Доступно только для заемщиков.</div>
  }

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    const newApp: Application = {
      id: Math.random().toString(),
      number: `APP-${Math.floor(Math.random() * 10000).toString().padStart(6, '0')}`,
      company_name: 'Альфа',
      amount: formData.amount,
      currency: formData.currency,
      target_country_code: formData.target_country_code,
      purpose: formData.purpose,
      term_months: Number(formData.term_months),
      status: 'moderation',
      created_at: new Date().toISOString(),
    }
    const updated = [newApp, ...applications]
    setApplications(updated)
    localStorage.setItem('mock_applications', JSON.stringify(updated))
    
    setIsCreateModalOpen(false)
    setFormData({ amount: '', currency: 'EUR', target_country_code: 'DE', purpose: '', term_months: '6' })
  }

  const handleAcceptOffer = (acceptedOffer: Offer) => {
    const updatedOffers = offers.map(o => {
      if (o.id === acceptedOffer.id) return { ...o, status: 'accepted' }
      if (o.application_id === acceptedOffer.application_id && o.status === 'sent') return { ...o, status: 'rejected' }
      return o
    })
    setOffers(updatedOffers)
    localStorage.setItem('mock_offers', JSON.stringify(updatedOffers))

    const updatedApps = applications.map(a => 
      a.id === acceptedOffer.application_id ? { ...a, status: 'agreed' as const } : a
    )
    setApplications(updatedApps)
    localStorage.setItem('mock_applications', JSON.stringify(updatedApps))

    const newDeal = {
      id: Math.random().toString(),
      application_number: selectedAppForOffers?.number,
      lender_name: acceptedOffer.company_name,
      amount: acceptedOffer.amount,
      currency: acceptedOffer.currency,
      interest_rate: acceptedOffer.interest_rate_annual,
      fee_percent: acceptedOffer.fee_percent,
      agreed_at: new Date().toISOString()
    }
    const existingDeals = JSON.parse(localStorage.getItem('mock_deals') || '[]')
    localStorage.setItem('mock_deals', JSON.stringify([newDeal, ...existingDeals]))

    addNotification({
      type: 'deal',
      title: `Ваш оффер по заявке ${selectedAppForOffers?.number} принят!`,
      description: `Заемщик принял ваше предложение. Сделка успешно зафиксирована.`,
      entity_id: acceptedOffer.application_id
    })

    alert(`Оффер принят! Сделка на сумму ${acceptedOffer.amount} ${acceptedOffer.currency} зафиксирована.`)
    setSelectedAppForOffers(null)
  }

  const currentAppOffers = offers.filter(o => o.application_id === selectedAppForOffers?.id)

  return (
    <div className="space-y-6 relative">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold text-gray-900">Мои заявки</h1>
        <Button onClick={() => setIsCreateModalOpen(true)}>+ Создать заявку</Button>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {applications.map((app) => {
            const appOffersCount = offers.filter(o => o.application_id === app.id).length
            
            return (
              <li key={app.id} className="p-4 hover:bg-gray-50">
                <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-sm text-gray-500">{app.number}</span>
                      {getStatusBadge(app.status)}
                    </div>
                    <p className="mt-1 text-sm font-medium text-gray-900">{app.purpose}</p>
                    <div className="mt-1 text-sm text-gray-500 flex gap-4">
                      <span>{app.amount} {app.currency}</span>
                      <span>Срок: {app.term_months} мес.</span>
                      <span>Страна: {app.target_country_code}</span>
                    </div>
                  </div>
                  <div>
                    <Button 
                      variant={appOffersCount > 0 && app.status === 'published' ? 'primary' : 'outline'}
                      onClick={() => setSelectedAppForOffers(app)}
                    >
                      Предложения ({appOffersCount})
                    </Button>
                  </div>
                </div>
              </li>
            )
          })}
        </ul>
      </div>

      {/* Модалка создания заявки */}
      {isCreateModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
            <h2 className="text-xl font-semibold mb-4">Создание заявки</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700">Сумма и валюта</label>
                <div className="mt-1 flex gap-2">
                  <input required type="number" min="1" step="0.01" value={formData.amount} onChange={e => setFormData({...formData, amount: e.target.value})} className="block w-full rounded-md border-gray-300 shadow-sm p-2 border" placeholder="500000.00" />
                  <select value={formData.currency} onChange={e => setFormData({...formData, currency: e.target.value})} className="rounded-md border-gray-300 shadow-sm p-2 border">
                    <option value="EUR">EUR</option><option value="USD">USD</option><option value="RUB">RUB</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Целевая юрисдикция</label>
                <select value={formData.target_country_code} onChange={e => setFormData({...formData, target_country_code: e.target.value})} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm p-2 border">
                  <option value="DE">Германия (DE)</option><option value="CN">Китай (CN)</option><option value="TR">Турция (TR)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Срок (месяцев)</label>
                <input required type="number" min="1" max="120" value={formData.term_months} onChange={e => setFormData({...formData, term_months: e.target.value})} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm p-2 border" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Назначение</label>
                <textarea required rows={3} value={formData.purpose} onChange={e => setFormData({...formData, purpose: e.target.value})} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm p-2 border" placeholder="Закупка оборудования..."></textarea>
              </div>
              <div className="flex justify-end gap-3 mt-6">
                <Button type="button" variant="outline" onClick={() => setIsCreateModalOpen(false)}>Отмена</Button>
                <Button type="submit">Отправить на модерацию</Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Модалка просмотра офферов */}
      {selectedAppForOffers && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-2xl rounded-lg bg-white p-6 shadow-xl max-h-[90vh] overflow-y-auto">
            <div className="flex justify-between items-start mb-4">
              <div>
                <h2 className="text-xl font-semibold">Предложения по заявке {selectedAppForOffers.number}</h2>
                <p className="text-sm text-gray-500">Запрошено: {selectedAppForOffers.amount} {selectedAppForOffers.currency}</p>
              </div>
              <Button variant="outline" onClick={() => setSelectedAppForOffers(null)}>Закрыть</Button>
            </div>

            {currentAppOffers.length === 0 ? (
              <div className="text-center py-8 text-gray-500">Пока нет предложений от кредиторов.</div>
            ) : (
              <div className="space-y-4">
                {currentAppOffers.map(offer => (
                  <div key={offer.id} className="border rounded-lg p-4 flex flex-col md:flex-row justify-between items-start md:items-center gap-4 bg-gray-50">
                    <div>
                      <div className="flex items-center gap-2 mb-1">
                        <span className="font-semibold text-gray-900">{offer.company_name}</span>
                        {offer.status === 'sent' && <Badge variant="warning">Активен</Badge>}
                        {offer.status === 'accepted' && <Badge variant="success">Принят</Badge>}
                        {offer.status === 'rejected' && <Badge variant="danger">Отклонен</Badge>}
                      </div>
                      <p className="text-sm text-gray-900">
                        Сумма: <span className="font-medium">{Number(offer.amount).toLocaleString('ru-RU')} {offer.currency}</span>
                      </p>
                      <p className="text-sm text-gray-600">
                        Ставка: <span className="font-medium">{offer.interest_rate_annual}%</span> | Комиссия: {offer.fee_percent}%
                      </p>
                    </div>
                    {/* ДОБАВЛЕНА КНОПКА ЧАТА РЯДОМ С ПРИНЯТЬ */}
                    <div className="flex gap-2">
                      <Button variant="outline" onClick={() => setChatOffer(offer)}>Чат</Button>
                      {offer.status === 'sent' && selectedAppForOffers.status === 'published' && (
                        <Button onClick={() => handleAcceptOffer(offer)}>Принять</Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      <ChatModal
        isOpen={!!chatOffer}
        onClose={() => setChatOffer(null)}
        offerId={chatOffer?.id || ''}
        offerNumber={chatOffer?.application_number || selectedAppForOffers?.number || ''}
        role="borrower"
      />
    </div>
  )
}
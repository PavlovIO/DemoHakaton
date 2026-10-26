import { useState } from 'react'
import { toast } from 'sonner'
import { useAuth } from '@/hooks/useAuth'
import { useCreateApplication, useMyApplications, useSubmitApplication, type Application } from '@/hooks/useApplications'
import { useAcceptOffer, useApplicationOffers, type Offer } from '@/hooks/useOffers'
import { handleApiError } from '@/lib/api-errors'
import { formatMoney, formatPercent, MAX_MONEY } from '@/lib/money'
import { LoadMore } from '@/components/ui/LoadMore'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { ChatModal } from '@/components/ChatModal'

const getStatusBadge = (status: Application['status']) => {
  switch (status) {
    case 'draft': return <Badge variant="default">Черновик</Badge>
    case 'moderation': return <Badge variant="warning">На модерации</Badge>
    case 'published': return <Badge variant="success">Опубликована</Badge>
    case 'agreed': return <Badge variant="agreed">Согласована</Badge>
    case 'rejected': return <Badge variant="danger">Отклонена</Badge>
    case 'cancelled': return <Badge variant="default">Отменена</Badge>
    default: return <Badge>{status}</Badge>
  }
}

const EMPTY_FORM = {
  amount: '',
  currency: 'EUR' as Application['currency'],
  target_country_code: 'DE',
  transfer_method: 'swift' as Application['transfer_method'],
  purpose: '',
  term_months: '6',
}

export function Applications() {
  const { user } = useAuth()

  // Проверка роли до запросов: кредитору GET /applications отдал бы всю витрину.
  if (user?.role !== 'borrower') {
    return <div className="p-6">Доступно только для заемщиков.</div>
  }
  return <BorrowerApplications />
}

function BorrowerApplications() {
  const applicationsQuery = useMyApplications()
  const { data: applications, isLoading } = applicationsQuery
  const createApplication = useCreateApplication()
  const submitApplication = useSubmitApplication()
  const acceptOffer = useAcceptOffer()

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [selectedAppForOffers, setSelectedAppForOffers] = useState<Application | null>(null)
  const { data: currentAppOffers = [] } = useApplicationOffers(selectedAppForOffers?.id)

  const [chatOffer, setChatOffer] = useState<Offer | null>(null)

  const [formData, setFormData] = useState(EMPTY_FORM)

  const handleSubmit = (id: string) => {
    submitApplication.mutate(id, {
      onSuccess: () => toast.success('Заявка отправлена на модерацию'),
      onError: handleApiError,
    })
  }

  // Черновик сохраняется сразу: если подача сорвётся, его можно подать из списка,
  // а повторное нажатие в форме не плодит дубликаты.
  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    createApplication.mutate(
      { ...formData, term_months: Number(formData.term_months) },
      {
        onSuccess: (draft) => {
          setIsCreateModalOpen(false)
          setFormData(EMPTY_FORM)
          handleSubmit(draft.id)
        },
        onError: handleApiError,
      },
    )
  }

  const handleAcceptOffer = (offer: Offer) => {
    acceptOffer.mutate(offer.id, {
      onSuccess: (deal) => {
        toast.success(`Сделка ${deal.number} на ${formatMoney(deal.amount, deal.currency)} зафиксирована`)
        setSelectedAppForOffers(null)
      },
      onError: handleApiError,
    })
  }

  return (
    <div className="space-y-6 relative">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold text-gray-900">Мои заявки</h1>
        <Button onClick={() => setIsCreateModalOpen(true)}>+ Создать заявку</Button>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {applications?.items.map((app) => {
            const appOffersCount = app.offers_count ?? 0
            
            return (
              <li key={app.id} className="p-4 hover:bg-gray-50">
                <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-sm text-gray-500">{app.number}</span>
                      {getStatusBadge(app.status)}
                    </div>
                    <p className="mt-1 text-sm font-medium text-gray-900">{app.purpose}</p>
                    {app.status === 'rejected' && app.moderation_comment && (
                      <p className="mt-1 text-sm text-danger">Причина отказа: {app.moderation_comment}</p>
                    )}
                    <div className="mt-1 text-sm text-gray-500 flex gap-4">
                      <span>{formatMoney(app.amount, app.currency)}</span>
                      <span>Срок: {app.term_months} мес.</span>
                      <span>Страна: {app.target_country_code}</span>
                    </div>
                  </div>
                  <div className="flex gap-2">
                    {app.status === 'draft' && (
                      <Button disabled={submitApplication.isPending} onClick={() => handleSubmit(app.id)}>
                        Отправить на модерацию
                      </Button>
                    )}
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
          {!isLoading && applications?.items.length === 0 && (
            <li className="p-8 text-center text-gray-500">Заявок пока нет.</li>
          )}
        </ul>
        <LoadMore query={applicationsQuery} />
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
                  <input required type="number" min="1" max={MAX_MONEY} step="0.01" value={formData.amount} onChange={e => setFormData({...formData, amount: e.target.value})} className="block w-full rounded-md border-gray-300 shadow-sm p-2 border" placeholder="500000.00" />
                  <select value={formData.currency} onChange={e => setFormData({...formData, currency: e.target.value as Application['currency']})} className="rounded-md border-gray-300 shadow-sm p-2 border">
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
                <label className="block text-sm font-medium text-gray-700">Способ перевода</label>
                <select value={formData.transfer_method} onChange={e => setFormData({...formData, transfer_method: e.target.value as Application['transfer_method']})} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm p-2 border">
                  <option value="swift">SWIFT</option><option value="sepa">SEPA</option><option value="other">Другое</option>
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
                <Button type="submit" disabled={createApplication.isPending}>Отправить на модерацию</Button>
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
                <p className="text-sm text-gray-500">Запрошено: {formatMoney(selectedAppForOffers.amount, selectedAppForOffers.currency)}</p>
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
                        <span className="font-semibold text-gray-900">{offer.lender.name}</span>
                        {offer.status === 'sent' && <Badge variant="warning">Активен</Badge>}
                        {offer.status === 'accepted' && <Badge variant="success">Принят</Badge>}
                        {offer.status === 'rejected' && <Badge variant="danger">Отклонен</Badge>}
                      </div>
                      <p className="text-sm text-gray-900">
                        Сумма: <span className="font-medium">{formatMoney(offer.amount, offer.currency)}</span>
                      </p>
                      <p className="text-sm text-gray-600">
                        Ставка: <span className="font-medium">{formatPercent(offer.interest_rate_annual)}</span> | Комиссия площадки: {formatPercent(offer.fee_percent)}
                      </p>
                      <p className="text-xs text-gray-400">Комиссия платформы фиксирована и не может быть изменена.</p>
                    </div>
                    <div className="flex gap-2">
                      <Button variant="outline" onClick={() => setChatOffer(offer)}>Чат</Button>
                      {offer.status === 'sent' && selectedAppForOffers.status === 'published' && (
                        <Button disabled={acceptOffer.isPending} onClick={() => handleAcceptOffer(offer)}>Принять</Button>
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
        key={chatOffer?.id}
        isOpen={!!chatOffer}
        onClose={() => setChatOffer(null)}
        offerId={chatOffer?.id || ''}
        offerNumber={chatOffer?.application_number || selectedAppForOffers?.number || ''}
        counterpartyName={chatOffer?.lender.name}
      />
    </div>
  )
}
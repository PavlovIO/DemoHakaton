import { useState } from 'react'
import { toast } from 'sonner'
import { useMarketplace, type Application } from '@/hooks/useApplications'
import { useCreateOffer, type OfferCreate } from '@/hooks/useOffers'
import { handleApiError } from '@/lib/api-errors'
import { formatMoney } from '@/lib/money'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'

export function Marketplace() {
  const [filterCountry, setFilterCountry] = useState('')
  const [filterCurrency, setFilterCurrency] = useState('')
  const [filterAmountMin, setFilterAmountMin] = useState('')

  // Фильтрует бэкенд; пустые значения в запрос не уходят.
  const { data } = useMarketplace({
    target_country: filterCountry || undefined,
    currency: filterCurrency || undefined,
    amount_min: filterAmountMin || undefined,
  })
  const filteredApps = data?.items ?? []
  const createOffer = useCreateOffer()

  const [selectedApp, setSelectedApp] = useState<Application | null>(null)
  const [offerData, setOfferData] = useState({
    amount: '',
    interest_rate_annual: '8.00',
    fee_percent: '1.00',
    repayment_schedule: 'bullet' as OfferCreate['repayment_schedule'],
    collateral: '',
    additional_terms: ''
  })

  const openOfferModal = (app: Application) => {
    setSelectedApp(app)
    setOfferData({
      amount: app.amount,
      interest_rate_annual: '8.00',
      fee_percent: '1.00',
      repayment_schedule: 'bullet',
      collateral: '',
      additional_terms: ''
    })
  }

  const handleCreateOffer = (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedApp) return

    createOffer.mutate(
      {
        applicationId: selectedApp.id,
        amount: offerData.amount,
        interest_rate_annual: offerData.interest_rate_annual,
        fee_percent: offerData.fee_percent || undefined,
        repayment_schedule: offerData.repayment_schedule,
        collateral: offerData.collateral || undefined,
        additional_terms: offerData.additional_terms || undefined,
      },
      {
        onSuccess: () => {
          toast.success('Оффер отправлен заемщику')
          setSelectedApp(null)
        },
        onError: handleApiError,
      },
    )
  }

  return (
    <div className="space-y-6 relative">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Витрина заявок</h1>
        <p className="mt-1 text-sm text-gray-500">Опубликованные заявки на привлечение ликвидности</p>
      </div>

      <div className="bg-white p-4 rounded-lg shadow border border-gray-200 grid grid-cols-1 md:grid-cols-4 gap-4">
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">Страна</label>
          <select value={filterCountry} onChange={e => setFilterCountry(e.target.value)} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm">
            <option value="">Все страны</option>
            <option value="DE">Германия (DE)</option>
            <option value="CN">Китай (CN)</option>
            <option value="TR">Турция (TR)</option>
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">Валюта</label>
          <select value={filterCurrency} onChange={e => setFilterCurrency(e.target.value)} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm">
            <option value="">Любая</option>
            <option value="EUR">EUR</option>
            <option value="USD">USD</option>
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">Сумма от</label>
          <input type="number" value={filterAmountMin} onChange={e => setFilterAmountMin(e.target.value)} placeholder="0" className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
        </div>
        <div className="flex items-end">
          <Button variant="outline" className="w-full" onClick={() => { setFilterCountry(''); setFilterCurrency(''); setFilterAmountMin(''); }}>Сбросить</Button>
        </div>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {filteredApps.map((app) => (
            <li key={app.id} className="p-4 hover:bg-gray-50 transition-colors">
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div className="flex-1">
                  <div className="flex items-center gap-3 mb-1">
                    <span className="font-mono text-sm text-gray-500">{app.number}</span>
                    <Badge variant="success">Опубликована</Badge>
                    <span className="text-sm font-semibold text-gray-900">{app.borrower.name}</span>
                  </div>
                  <p className="text-sm text-gray-900">{app.purpose}</p>
                  <div className="mt-2 flex flex-wrap gap-4 text-sm text-gray-500">
                    <span className="font-medium text-gray-900">{formatMoney(app.amount, app.currency)}</span>
                    <span>Срок: {app.term_months} мес.</span>
                    <span>Страна: {app.target_country_code}</span>
                  </div>
                </div>
                <div>
                  {app.my_offer_id ? (
                    <Badge variant="success">Оффер отправлен</Badge>
                  ) : (
                    <Button onClick={() => openOfferModal(app)}>Предложить финансирование</Button>
                  )}
                </div>
              </div>
            </li>
          ))}
          {filteredApps.length === 0 && (
            <li className="p-8 text-center text-gray-500">По вашему запросу ничего не найдено.</li>
          )}
        </ul>
      </div>

      {/* Модалка создания оффера */}
      {selectedApp && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl max-h-[90vh] overflow-y-auto">
            <div className="mb-4">
              <h2 className="text-xl font-semibold">Предложение по заявке {selectedApp.number}</h2>
              <p className="text-sm text-gray-500 mt-1">{selectedApp.borrower.name} запрашивает {formatMoney(selectedApp.amount, selectedApp.currency)}</p>
            </div>
            
            <form onSubmit={handleCreateOffer} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Сумма ({selectedApp.currency})</label>
                  <input required type="number" max={selectedApp.amount} step="0.01" value={offerData.amount} onChange={e => setOfferData({...offerData, amount: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Ставка (% годовых)</label>
                  <input required type="number" step="0.1" value={offerData.interest_rate_annual} onChange={e => setOfferData({...offerData, interest_rate_annual: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Комиссия за организацию (%)</label>
                  <input type="number" step="0.1" value={offerData.fee_percent} onChange={e => setOfferData({...offerData, fee_percent: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">График погашения</label>
                  <select value={offerData.repayment_schedule} onChange={e => setOfferData({...offerData, repayment_schedule: e.target.value as OfferCreate['repayment_schedule']})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm">
                    <option value="bullet">В конце срока (Bullet)</option>
                    <option value="annuity">Аннуитетный</option>
                    <option value="interest_monthly_principal_bullet">Проценты ежемесячно, тело в конце</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Обеспечение (опционально)</label>
                <input type="text" value={offerData.collateral} onChange={e => setOfferData({...offerData, collateral: e.target.value})} placeholder="Например: залог оборудования" className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Доп. условия</label>
                <textarea rows={2} value={offerData.additional_terms} onChange={e => setOfferData({...offerData, additional_terms: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm"></textarea>
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t">
                <Button type="button" variant="outline" onClick={() => setSelectedApp(null)}>Отмена</Button>
                <Button type="submit" disabled={createOffer.isPending}>Отправить предложение</Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
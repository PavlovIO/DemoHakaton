import { useState } from 'react'
import { useQueryClient } from "@tanstack/react-query"
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/hooks/useAuth'
import { useMarketplace, type Application } from '@/hooks/useApplications'
import { useCreateOffer, type OfferCreate } from '@/hooks/useOffers'
import { useCountries, useCurrencies } from '@/hooks/useDictionaries'
import type { components } from '@/lib/api-schema'
import { NOTIFICATIONS_KEY } from "@/hooks/useNotifications"
import { handleApiError } from '@/lib/api-errors'
import { formatMoney, MAX_MONEY, MAX_PERCENT } from '@/lib/money'
import {
  applicationStatusLabel,
  countryName,
  currencyName,
  REPAYMENT_SCHEDULES,
  repaymentScheduleLabel,
} from '@/lib/labels'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { LoadMore } from '@/components/ui/LoadMore'

type CountryCode = components['schemas']['CountryCode']

export function Marketplace() {
  const { user } = useAuth()

  // Проверка роли до запросов: заёмщику GET /applications отдал бы его собственные заявки как витрину.
  if (user?.role !== 'lender') {
    return <div className="p-6">Доступно только для кредиторов.</div>
  }
  return <LenderMarketplace />
}

function LenderMarketplace() {
  const { t } = useTranslation()
  const [filterCountry, setFilterCountry] = useState('')
  const [filterCurrency, setFilterCurrency] = useState('')
  const [filterAmountMin, setFilterAmountMin] = useState('')

  const countriesQuery = useCountries()
  const currenciesQuery = useCurrencies()
  // Справочник приходит отсортированным по name_ru — на английском пересортировываем.
  const countryOptions = [...(countriesQuery.data ?? [])].sort((a, b) =>
    countryName(a).localeCompare(countryName(b)),
  )
  const currencyOptions = currenciesQuery.data ?? []

  // Фильтрует бэкенд; пустые значения в запрос не уходят. Сумма без валюты не фильтрует.
  const target_country = filterCountry ? [filterCountry as CountryCode] : undefined
  const marketplaceQuery = useMarketplace(
    filterCurrency
      ? { target_country, currency: filterCurrency, amount_min: filterAmountMin || undefined }
      : { target_country },
  )
  const { data, isLoading } = marketplaceQuery
  const filteredApps = data?.items ?? []
  const createOffer = useCreateOffer()
  const queryClient = useQueryClient()

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
          void queryClient.invalidateQueries({ queryKey: NOTIFICATIONS_KEY })
          toast.success(t('marketplace.offer.created'))
          setSelectedApp(null)
        },
        onError: handleApiError,
      },
    )
  }

  return (
    <div className="space-y-6 relative">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">{t('marketplace.title')}</h1>
        <p className="mt-1 text-sm text-gray-500">{t('marketplace.subtitle')}</p>
      </div>

      <div className="bg-white p-4 rounded-lg shadow border border-gray-200 grid grid-cols-1 md:grid-cols-4 gap-4">
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">{t('marketplace.country')}</label>
          <select value={filterCountry} onChange={e => setFilterCountry(e.target.value)} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm">
            <option value="">{t('marketplace.allCountries')}</option>
            {countryOptions.map((c) => (
              <option key={c.code} value={c.code}>
                {countryName(c)} ({c.code}){c.fatf_high_risk ? ` — ${t('marketplace.fatfHighRisk')}` : ''}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">{t('marketplace.currency')}</label>
          <select value={filterCurrency} onChange={e => setFilterCurrency(e.target.value)} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm">
            <option value="">{t('marketplace.anyCurrency')}</option>
            {currencyOptions.map((c) => (
              <option key={c.code} value={c.code}>{c.code} — {currencyName(c)}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">{t('marketplace.amountFrom')}</label>
          <input type="number" value={filterAmountMin} onChange={e => setFilterAmountMin(e.target.value)} disabled={!filterCurrency} placeholder={filterCurrency ? '0' : t('marketplace.chooseCurrencyFirst')} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm disabled:bg-gray-100" />
        </div>
        <div className="flex items-end">
          <Button variant="outline" className="w-full" onClick={() => { setFilterCountry(''); setFilterCurrency(''); setFilterAmountMin(''); }}>{t('marketplace.reset')}</Button>
        </div>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {filteredApps.map((app) => (
            <li key={app.id} className="p-4 hover:bg-gray-50 transition-colors">
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <div className="mb-1 flex flex-wrap items-center gap-x-3 gap-y-1">
                    <span className="font-mono text-sm text-gray-500">{app.number}</span>
                    <Badge variant="success">{applicationStatusLabel('published')}</Badge>
                    <span className="break-words text-sm font-semibold text-gray-900">{app.borrower.name}</span>
                  </div>
                  <p className="break-words text-sm text-gray-900">{app.purpose}</p>
                  <div className="mt-2 flex flex-wrap gap-4 text-sm text-gray-500">
                    <span className="font-medium text-gray-900">{formatMoney(app.amount, app.currency)}</span>
                    <span>{t('marketplace.term', { months: app.term_months })}</span>
                    <span>{t('marketplace.targetCountry', { code: app.target_country_code })}</span>
                  </div>
                </div>
                <div className="sm:shrink-0">
                  {app.my_offer_id ? (
                    <Badge variant="success">{t('marketplace.offerSent')}</Badge>
                  ) : (
                    <Button className="w-full sm:w-auto" onClick={() => openOfferModal(app)}>{t('marketplace.makeOffer')}</Button>
                  )}
                </div>
              </div>
            </li>
          ))}
          {!isLoading && filteredApps.length === 0 && (
            <li className="p-8 text-center text-gray-500">{t('marketplace.empty')}</li>
          )}
        </ul>
        <LoadMore query={marketplaceQuery} />
      </div>

      {/* Модалка создания оффера */}
      {selectedApp && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl max-h-[90vh] overflow-y-auto">
            <div className="mb-4">
              <h2 className="text-xl font-semibold">{t('marketplace.offer.title', { number: selectedApp.number })}</h2>
              <p className="text-sm text-gray-500 mt-1">{t('marketplace.offer.requests', { borrower: selectedApp.borrower.name, amount: formatMoney(selectedApp.amount, selectedApp.currency) })}</p>
            </div>
            
            <form onSubmit={handleCreateOffer} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">{t('marketplace.offer.amount', { currency: selectedApp.currency })}</label>
                  <input required type="number" min="1" max={Math.min(parseFloat(selectedApp.amount), MAX_MONEY)} step="0.01" value={offerData.amount} onChange={e => setOfferData({...offerData, amount: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">{t('marketplace.offer.rate')}</label>
                  <input required type="number" min="0" max={MAX_PERCENT} step="0.1" value={offerData.interest_rate_annual} onChange={e => setOfferData({...offerData, interest_rate_annual: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">{t('marketplace.offer.fee')}</label>
                  <input type="number" min="0" max={MAX_PERCENT} step="0.1" value={offerData.fee_percent} onChange={e => setOfferData({...offerData, fee_percent: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
                  <p className="mt-1 text-xs text-gray-500">{t('marketplace.offer.feeFixed')}</p>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">{t('marketplace.offer.schedule')}</label>
                  <select value={offerData.repayment_schedule} onChange={e => setOfferData({...offerData, repayment_schedule: e.target.value as OfferCreate['repayment_schedule']})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm">
                    {REPAYMENT_SCHEDULES.filter((v) => v !== 'custom').map((v) => (
                      <option key={v} value={v}>{repaymentScheduleLabel(v)}</option>
                    ))}
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('marketplace.offer.collateral')}</label>
                <input type="text" value={offerData.collateral} onChange={e => setOfferData({...offerData, collateral: e.target.value})} placeholder={t('marketplace.offer.collateralPlaceholder')} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm" />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('marketplace.offer.terms')}</label>
                <textarea rows={2} value={offerData.additional_terms} onChange={e => setOfferData({...offerData, additional_terms: e.target.value})} className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm"></textarea>
              </div>

              <div className="flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-end sm:gap-3">
                <Button type="button" variant="outline" className="w-full text-wrap sm:w-auto" onClick={() => setSelectedApp(null)}>{t('common.cancel')}</Button>
                <Button type="submit" className="w-full text-wrap sm:w-auto" disabled={createOffer.isPending}>{t('marketplace.offer.submit')}</Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
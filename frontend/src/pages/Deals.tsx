import { useTranslation } from 'react-i18next'
import { useAuth } from '@/hooks/useAuth'
import { useDeals } from '@/hooks/useDeals'
import { formatMoney, formatPercent } from '@/lib/money'
import { Badge } from '@/components/ui/Badge'
import { LoadMore } from '@/components/ui/LoadMore'
import { currentLocale } from '@/i18n'

export function Deals() {
  const { user } = useAuth()
  const { t } = useTranslation()
  const dealsQuery = useDeals()
  const { data, isLoading } = dealsQuery
  const deals = data?.items ?? []

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-gray-900">{t('deals.title')}</h1>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {deals.map((deal) => (
            <li key={deal.id} className="p-4 hover:bg-gray-50">
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div>
                  <div className="mb-1 flex flex-wrap items-center gap-3">
                    <span className="font-mono text-sm text-gray-500">{deal.number}</span>
                    <Badge variant="success">{t('deals.agreed')}</Badge>
                  </div>
                  <p className="text-sm font-medium text-gray-900">
                    {user?.role === 'borrower'
                      ? t('deals.lender', { name: deal.lender.name })
                      : t('deals.borrower', { name: deal.borrower.name })}
                  </p>
                  <div className="mt-2 flex flex-wrap gap-4 text-sm text-gray-600">
                    <span className="font-medium text-gray-900">{formatMoney(deal.amount, deal.currency)}</span>
                    <span>{t('deals.rate', { rate: formatPercent(deal.interest_rate_annual) })}</span>
                    <span>{t('deals.fee', { fee: formatPercent(deal.fee_percent) })}</span>
                    <span>{t('deals.date', { date: new Date(deal.agreed_at).toLocaleDateString(currentLocale()) })}</span>
                  </div>
                </div>
              </div>
            </li>
          ))}
          {!isLoading && deals.length === 0 && (
            <li className="p-8 text-center text-gray-500">
              {t('deals.empty')}
            </li>
          )}
        </ul>
        <LoadMore query={dealsQuery} />
      </div>
    </div>
  )
}
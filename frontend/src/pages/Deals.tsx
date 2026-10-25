import { useAuth } from '@/hooks/useAuth'
import { useDeals } from '@/hooks/useDeals'
import { formatMoney, formatPercent } from '@/lib/money'
import { Badge } from '@/components/ui/Badge'

export function Deals() {
  const { user } = useAuth()
  const { data, isLoading } = useDeals()
  const deals = data?.items ?? []

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold text-gray-900">Сделки</h1>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {deals.map((deal) => (
            <li key={deal.id} className="p-4 hover:bg-gray-50">
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div>
                  <div className="flex items-center gap-3 mb-1">
                    <span className="font-mono text-sm text-gray-500">{deal.number}</span>
                    <Badge variant="success">Сделка зафиксирована</Badge>
                  </div>
                  <p className="text-sm font-medium text-gray-900">
                    {user?.role === 'borrower' ? `Кредитор: ${deal.lender.name}` : `Заемщик: ${deal.borrower.name}`}
                  </p>
                  <div className="mt-2 flex flex-wrap gap-4 text-sm text-gray-600">
                    <span className="font-medium text-gray-900">{formatMoney(deal.amount, deal.currency)}</span>
                    <span>Ставка: {formatPercent(deal.interest_rate_annual)}</span>
                    <span>Комиссия: {formatPercent(deal.fee_percent)}</span>
                    <span>Дата: {new Date(deal.agreed_at).toLocaleDateString('ru-RU')}</span>
                  </div>
                </div>
              </div>
            </li>
          ))}
          {!isLoading && deals.length === 0 && (
            <li className="p-8 text-center text-gray-500">
              Сделок пока нет.
            </li>
          )}
        </ul>
      </div>
    </div>
  )
}
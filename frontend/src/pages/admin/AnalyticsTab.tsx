import { useAnalyticsSummary } from '@/hooks/useAdmin'
import { useCountries } from '@/hooks/useDictionaries'
import { applicationStatusLabel, countryLabel } from '@/lib/labels'
import { formatMoney } from '@/lib/money'
import type { components } from '@/lib/api-schema'

type Schemas = components['schemas']

const APPLICATION_STATUS_ORDER: Schemas['ApplicationStatus'][] = [
  'draft',
  'moderation',
  'rejected',
  'published',
  'agreed',
  'cancelled',
]

const APPLICATION_STATUS_BAR_COLOR: Record<Schemas['ApplicationStatus'], string> = {
  draft: 'bg-gray-400',
  moderation: 'bg-warning',
  rejected: 'bg-danger',
  published: 'bg-info',
  agreed: 'bg-success',
  cancelled: 'bg-gray-300',
}

function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="bg-white shadow sm:rounded-md p-5">
      <dt className="text-sm font-medium text-gray-500">{label}</dt>
      <dd className="mt-1 text-3xl font-semibold tracking-tight text-gray-900">
        {value.toLocaleString('ru-RU')}
      </dd>
    </div>
  )
}


function StatusBars({ counts }: { counts: Record<string, number> }) {
  const rows = APPLICATION_STATUS_ORDER.map((status) => ({
    status,
    count: counts[status] ?? 0,
  }))
  const extra = Object.entries(counts)
    .filter(([status]) => !(APPLICATION_STATUS_ORDER as string[]).includes(status))
    .map(([status, count]) => ({ status: status as Schemas['ApplicationStatus'], count }))
  const max = Math.max(1, ...rows.map((r) => r.count), ...extra.map((r) => r.count))

  return (
    <ul className="space-y-2">
      {[...rows, ...extra].map(({ status, count }) => (
        <li key={status} className="flex items-center gap-3 text-sm">
          <span className="w-40 shrink-0 text-gray-600">{applicationStatusLabel(status)}</span>
          <div className="h-4 flex-1 rounded bg-gray-100">
            <div
              className={`h-4 rounded ${APPLICATION_STATUS_BAR_COLOR[status] ?? 'bg-gray-400'}`}
              style={{ width: `${Math.round((count / max) * 100)}%` }}
              role="progressbar"
              aria-valuenow={count}
              aria-valuemin={0}
              aria-valuemax={max}
              aria-label={applicationStatusLabel(status)}
            />
          </div>
          <span className="w-12 shrink-0 text-right font-medium text-gray-900">{count}</span>
        </li>
      ))}
    </ul>
  )
}

function CountryTop({ title, stats, countryNames }: {
  title: string
  stats: Schemas['CountryStat'][]
  countryNames?: { code: string; name_ru: string }[]
}) {
  const max = Math.max(1, ...stats.map((s) => s.count))
  return (
    <div className="bg-white shadow sm:rounded-md p-5">
      <h3 className="mb-3 text-sm font-medium text-gray-500">{title}</h3>
      {stats.length === 0 ? (
        <p className="text-sm text-gray-400">Нет данных.</p>
      ) : (
        <ul className="space-y-2">
          {stats.map((s) => (
            <li key={s.country_code} className="flex items-center gap-3 text-sm">
              <span className="w-48 shrink-0 truncate text-gray-600" title={s.country_code}>
                {countryLabel(s.country_code, countryNames)}
              </span>
              <div className="h-4 flex-1 rounded bg-gray-100">
                <div className="h-4 rounded bg-primary-500" style={{ width: `${Math.round((s.count / max) * 100)}%` }} />
              </div>
              <span className="w-12 shrink-0 text-right font-medium text-gray-900">{s.count}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// Вкладка «Аналитика»: счётчики, бары по статусам заявок, суммы по валютам,
// топы стран спроса/предложения. Графики — чистый Tailwind, новых зависимостей нет.
export function AnalyticsTab() {
  const summaryQuery = useAnalyticsSummary({ top_n: 5 })
  const { data: countries } = useCountries()
  const summary = summaryQuery.data

  if (summaryQuery.isLoading) {
    return <div className="p-6 text-center text-gray-500">Загрузка аналитики…</div>
  }
  if (!summary) {
    return <div className="p-6 text-center text-gray-500">Не удалось получить аналитику.</div>
  }

  return (
    <div className="space-y-6">
      {/* Счётчики */}
      <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="Заявки всего" value={summary.applications_total} />
        <StatCard label="На модерации" value={summary.applications_by_status.moderation ?? 0} />
        <StatCard label="Офферы всего" value={summary.offers_total} />
        <StatCard label="Сделки всего" value={summary.deals_total} />
      </dl>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Бары по статусам заявок */}
        <div className="bg-white shadow sm:rounded-md p-5">
          <h3 className="mb-3 text-sm font-medium text-gray-500">Заявки по статусам</h3>
          <StatusBars counts={summary.applications_by_status} />
        </div>

        {/* Суммы по валютам */}
        <div className="bg-white shadow sm:rounded-md p-5">
          <h3 className="mb-3 text-sm font-medium text-gray-500">Суммы по валютам</h3>
          {summary.amounts_by_currency.length === 0 ? (
            <p className="text-sm text-gray-400">Нет данных.</p>
          ) : (
            <table className="min-w-full text-sm">
              <thead>
                <tr className="text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                  <th className="pb-2">Валюта</th>
                  <th className="pb-2 text-right">Кол-во</th>
                  <th className="pb-2 text-right">Сумма</th>
                  <th className="pb-2 text-right">Средний чек</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {summary.amounts_by_currency.map((c: Schemas['CurrencyStat']) => (
                  <tr key={c.currency}>
                    <td className="py-2 font-medium text-gray-900">{c.currency}</td>
                    <td className="py-2 text-right text-gray-600">{c.count.toLocaleString('ru-RU')}</td>
                    <td className="py-2 text-right text-gray-600">{formatMoney(c.total_amount, c.currency)}</td>
                    <td className="py-2 text-right text-gray-600">{formatMoney(c.average_amount, c.currency)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <CountryTop title="Топ стран спроса (юрисдикции заявок)" stats={summary.top_demand_countries} countryNames={countries} />
        <CountryTop title="Топ стран предложения (кредиторы)" stats={summary.top_supply_countries} countryNames={countries} />
      </div>
    </div>
  )
}
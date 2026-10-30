import { useTranslation } from 'react-i18next'
import { useMyApplications, type Application } from '@/hooks/useApplications'
import { useDeals } from '@/hooks/useDeals'
import { useNotifications } from '@/hooks/useNotifications'
import { formatMoney } from '@/lib/money'
import { currentLocale } from '@/i18n'
import {
  DashboardSection,
  EmptyRow,
  KycBanner,
  ListRow,
  LoadingRows,
  StatCard,
  StatusBadge,
} from './common'

// Верхние пороги списков: счётчики KPI считают бэкенд по всем страницам, а здесь — только витрина.
const TOP_ROWS = 5

export function BorrowerDashboard({ kycStatus, rejectReason }: { kycStatus?: string; rejectReason?: string | null }) {
  const { t } = useTranslation()

  // Данные берутся из кэша тех же запросов, что и страницы «Мои заявки» и «Сделки»:
  // дашборд не создаёт отдельных эндпоинтов и живёт на их интервале опроса (30 с).
  const applicationsQuery = useMyApplications()
  const dealsQuery = useDeals()
  const notificationsQuery = useNotifications()

  const applications = applicationsQuery.data?.items ?? []
  const deals = dealsQuery.data?.items ?? []
  const notifications = notificationsQuery.data?.items ?? []

  const activeCount = applications.filter((a) => a.status === 'published' || a.status === 'moderation').length
  const publishedCount = applications.filter((a) => a.status === 'published').length
  const offersAwaiting = applications.reduce((sum, a) => sum + (a.offers_count ?? 0), 0)
  const totalRequested = applications
    .filter((a) => a.status === 'published' || a.status === 'moderation')
    .reduce((sum, a) => sum + Number(a.amount), 0)
  const currency = applications.find((a) => a.status === 'published' || a.status === 'moderation')?.currency

  const recentApplications = [...applications]
    .sort(compareByUpdatedDesc)
    .slice(0, TOP_ROWS)
  const withOffers = applications
    .filter((a) => a.status === 'published' && (a.offers_count ?? 0) > 0)
    .sort(compareByUpdatedDesc)
    .slice(0, TOP_ROWS)

  return (
    <div className="space-y-6">
      {kycStatus && kycStatus !== 'verified' && (
        <KycBanner status={kycStatus as never} rejectReason={rejectReason} />
      )}

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label={t('dashboard.borrower.activeApps')} value={activeCount} loading={applicationsQuery.isLoading} />
        <StatCard label={t('dashboard.borrower.published')} value={publishedCount} loading={applicationsQuery.isLoading} />
        <StatCard label={t('dashboard.borrower.offersAwaiting')} value={offersAwaiting} loading={applicationsQuery.isLoading} />
        <StatCard
          label={t('dashboard.borrower.requestedSum')}
          value={formatMoney(totalRequested.toFixed(2), currency)}
          loading={applicationsQuery.isLoading}
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-2">
        <DashboardSection
          title={t('dashboard.borrower.recentAppsTitle')}
          to="/applications"
          linkLabel={t('dashboard.viewAll')}
        >
          {applicationsQuery.isLoading && <LoadingRows />}
          {!applicationsQuery.isLoading && recentApplications.length === 0 && (
            <EmptyRow>{t('dashboard.borrower.recentAppsEmpty')}</EmptyRow>
          )}
          {recentApplications.map((app) => (
            <ApplicationRow key={app.id} app={app} />
          ))}
        </DashboardSection>

        <DashboardSection
          title={t('dashboard.borrower.offersTitle')}
          to="/applications"
          linkLabel={t('dashboard.viewAll')}
        >
          {applicationsQuery.isLoading && <LoadingRows />}
          {!applicationsQuery.isLoading && withOffers.length === 0 && (
            <EmptyRow>{t('dashboard.borrower.offersEmpty')}</EmptyRow>
          )}
          {withOffers.map((app) => (
            <ListRow
              key={app.id}
              title={t('dashboard.borrower.offersRow', { number: app.number, count: app.offers_count ?? 0 })}
              meta={formatMoney(app.amount, app.currency)}
              badge={<StatusBadge status={app.status} />}
              actionTo="/applications"
              actionLabel={t('dashboard.borrower.reviewAction')}
            />
          ))}
        </DashboardSection>

        <DashboardSection title={t('dashboard.recentDealsTitle')} to="/deals" linkLabel={t('dashboard.viewAll')}>
          {dealsQuery.isLoading && <LoadingRows />}
          {!dealsQuery.isLoading && deals.length === 0 && <EmptyRow>{t('deals.empty')}</EmptyRow>}
          {deals.slice(0, TOP_ROWS).map((deal) => (
            <ListRow
              key={deal.id}
              title={deal.number}
              meta={t('dashboard.dealMeta', {
                amount: formatMoney(deal.amount, deal.currency),
                date: new Date(deal.agreed_at).toLocaleDateString(currentLocale()),
              })}
              actionTo="/deals"
              actionLabel={t('dashboard.dealAction')}
            />
          ))}
        </DashboardSection>

        <DashboardSection title={t('dashboard.notificationsTitle')} to="/profile" linkLabel={t('dashboard.viewAll')}>
          {notificationsQuery.isLoading && <LoadingRows />}
          {!notificationsQuery.isLoading && notifications.length === 0 && (
            <EmptyRow>{t('dashboard.notificationsEmpty')}</EmptyRow>
          )}
          {notifications.slice(0, TOP_ROWS).map((n) => (
            <li key={n.id} className={`px-4 py-3 text-sm ${n.is_read ? 'text-gray-500' : 'font-medium text-gray-900'}`}>
              {n.title}
              <span className="mt-0.5 block text-xs font-normal text-gray-400">
                {new Date(n.created_at).toLocaleString(currentLocale())}
              </span>
            </li>
          ))}
        </DashboardSection>
      </div>
    </div>
  )
}

function compareByUpdatedDesc(a: Application, b: Application): number {
  return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
}

function ApplicationRow({ app }: { app: Application }) {
  const { t } = useTranslation()
  return (
    <ListRow
      title={`${app.number} · ${app.purpose}`}
      meta={t('dashboard.applicationMeta', {
        amount: formatMoney(app.amount, app.currency),
        term: app.term_months,
      })}
      badge={<StatusBadge status={app.status} />}
      actionTo="/applications"
      actionLabel={t('dashboard.openAction')}
    />
  )
}

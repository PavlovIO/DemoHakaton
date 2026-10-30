import { useTranslation } from 'react-i18next'
import { useDeals } from '@/hooks/useDeals'
import { useMarketplace, type Application } from '@/hooks/useApplications'
import { useMyOffers, OFFERS_KEY, type Offer, type OfferPage } from '@/hooks/useOffers'
import { useNotifications } from '@/hooks/useNotifications'
import { formatMoney, formatPercent } from '@/lib/money'
import { offerStatusLabel } from '@/lib/labels'
import { currentLocale } from '@/i18n'
import { Badge } from '@/components/ui/Badge'
import { useListTotal } from './useCounters'
import {
  DashboardSection,
  EmptyRow,
  KycBanner,
  ListRow,
  LoadingRows,
  StatCard,
  StatusBadge,
} from './common'
import { useDashboardKyc } from './useDashboardKyc'

// Витрина дашборда — топ newest; точные счётчики KPI берутся из total count-запросов.
const TOP_ROWS = 5

export function LenderDashboard() {
  const { t } = useTranslation()
  const { status: kycStatus, rejectReason } = useDashboardKyc()

  // Те же запросы, что у страниц «Витрина», «Мои предложения» и «Сделки», — без новых эндпоинтов.
  const marketplaceQuery = useMarketplace()
  const offersQuery = useMyOffers()
  const sentCountQuery = useListTotal<OfferPage>(OFFERS_KEY, '/offers', { status: ['sent'] })
  const dealsQuery = useDeals()
  const notificationsQuery = useNotifications()

  const marketplace = marketplaceQuery.data?.items ?? []
  const offers = offersQuery.data?.items ?? []
  const deals = dealsQuery.data?.items ?? []
  const notifications = notificationsQuery.data?.items ?? []

  const sentOffers = offers.filter((o) => o.status === 'sent')
  const unreadChats = offers.reduce((sum, o) => sum + (o.unread_messages ?? 0), 0)
  const activeVolume = sentOffers.reduce((sum, o) => sum + Number(o.amount), 0)
  const activeCurrency = sentOffers[0]?.currency

  return (
    <div className="space-y-6">
      {kycStatus && kycStatus !== 'verified' && <KycBanner status={kycStatus} rejectReason={rejectReason} />}

      <dl className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label={t('dashboard.lender.marketplace')}
          value={marketplaceQuery.data?.total ?? 0}
          loading={marketplaceQuery.isLoading}
        />
        <StatCard
          label={t('dashboard.lender.activeOffers')}
          value={sentCountQuery.data?.total ?? null}
          loading={sentCountQuery.isPending}
        />
        <StatCard
          label={t('dashboard.lender.activeVolume')}
          // formatMoney при пустом значении вернёт '—', а карточка ждёт число — грузим скелетон.
          value={activeVolume}
          loading={offersQuery.isLoading || activeCurrency === undefined}
        />
        <StatCard
          label={t('dashboard.lender.deals')}
          value={dealsQuery.data?.total ?? 0}
          loading={dealsQuery.isLoading}
        />
      </dl>

      <div className="grid gap-6 xl:grid-cols-2">
        <DashboardSection title={t('dashboard.lender.newAppsTitle')} to="/marketplace" linkLabel={t('dashboard.viewAll')}>
          {marketplaceQuery.isLoading && <LoadingRows />}
          {!marketplaceQuery.isLoading && marketplace.length === 0 && (
            <EmptyRow>{t('dashboard.lender.newAppsEmpty')}</EmptyRow>
          )}
          {marketplace.slice(0, TOP_ROWS).map((app) => (
            <MarketplaceRow key={app.id} app={app} />
          ))}
        </DashboardSection>

        <DashboardSection title={t('dashboard.lender.attentionTitle')} to="/offers" linkLabel={t('dashboard.viewAll')}>
          {offersQuery.isLoading && <LoadingRows />}
          {!offersQuery.isLoading && sentOffers.length === 0 && (
            <EmptyRow>{t('dashboard.lender.attentionEmpty')}</EmptyRow>
          )}
          {sentOffers.slice(0, TOP_ROWS).map((offer) => (
            <OfferRow key={offer.id} offer={offer} />
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

        <DashboardSection
          title={t('dashboard.notificationsTitle', { count: notificationsQuery.data?.unread_total ?? 0 })}
          to="/marketplace"
          linkLabel={t('dashboard.viewAll')}
        >
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

      {unreadChats > 0 && (
        <p className="text-sm text-gray-500">{t('dashboard.lender.unreadHint', { count: unreadChats })}</p>
      )}
    </div>
  )
}

function MarketplaceRow({ app }: { app: Application }) {
  const { t } = useTranslation()
  const hasMyOffer = !!app.my_offer_id
  return (
    <ListRow
      title={`${app.number} · ${app.purpose}`}
      meta={t('dashboard.applicationMeta', {
        amount: formatMoney(app.amount, app.currency),
        term: app.term_months,
      })}
      badge={
        hasMyOffer ? (
          <Badge variant="default">{t('marketplace.offerSent')}</Badge>
        ) : (
          <StatusBadge status={app.status} />
        )
      }
      actionTo="/marketplace"
      actionLabel={hasMyOffer ? t('dashboard.openAction') : t('marketplace.makeOffer')}
    />
  )
}

function OfferRow({ offer }: { offer: Offer }) {
  const { t } = useTranslation()
  const needsReply = (offer.unread_messages ?? 0) > 0
  return (
    <ListRow
      title={t('dashboard.lender.offerRow', { number: offer.application_number })}
      meta={t('dashboard.lender.offerMeta', {
        amount: formatMoney(offer.amount, offer.currency),
        rate: formatPercent(offer.interest_rate_annual),
      })}
      badge={
        <>
          <Badge variant={needsReply ? 'info' : 'warning'}>{offerStatusLabel(offer.status)}</Badge>
          {needsReply && (
            <Badge variant="danger">{t('dashboard.lender.unreadBadge', { count: offer.unread_messages ?? 0 })}</Badge>
          )}
        </>
      }
      actionTo="/offers"
      actionLabel={t('dashboard.openAction')}
    />
  )
}

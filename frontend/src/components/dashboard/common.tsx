import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, CheckCircle2, Clock } from 'lucide-react'
import type { components } from '@/lib/api-schema'
import { Badge } from '@/components/ui/Badge'
import { applicationStatusLabel } from '@/lib/labels'
import { cn } from '@/lib/utils'

type ApplicationStatus = components['schemas']['ApplicationStatus']
type KycStatus = components['schemas']['KycStatus']

const APPLICATION_BADGE_VARIANT: Record<ApplicationStatus, 'default' | 'success' | 'warning' | 'danger' | 'info'> = {
  draft: 'default',
  moderation: 'warning',
  rejected: 'danger',
  published: 'info',
  agreed: 'success',
  cancelled: 'default',
}

// Карточка KPI: подпись сверху, крупное число, при загрузке — скелетон вместо нуля.
export function StatCard({ label, value, loading }: { label: string; value: number | null; loading: boolean }) {
  return (
    <div className="bg-white shadow sm:rounded-md p-5">
      <dt className="text-sm font-medium text-gray-500">{label}</dt>
      <dd className="mt-1 text-3xl font-semibold tracking-tight text-gray-900">
        {loading ? (
          <span className="inline-block h-9 w-16 animate-pulse rounded bg-gray-200" aria-hidden />
        ) : (
          value
        )}
      </dd>
    </div>
  )
}

// Панель со списком элементов и ссылкой «смотреть все» в шапке.
export function DashboardSection({
  title,
  to,
  linkLabel,
  children,
}: {
  title: string
  to: string
  linkLabel: string
  children: ReactNode
}) {
  return (
    <div className="bg-white shadow overflow-hidden sm:rounded-md">
      <div className="flex items-center justify-between border-b border-gray-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
        <Link to={to} className="text-sm text-primary-600 hover:text-primary-700">
          {linkLabel}
        </Link>
      </div>
      <ul role="list" className="divide-y divide-gray-200">
        {children}
      </ul>
    </div>
  )
}

export function EmptyRow({ children }: { children: ReactNode }) {
  return <li className="px-4 py-6 text-center text-sm text-gray-500">{children}</li>
}

export function LoadingRows({ rows = 3 }: { rows?: number }) {
  return (
    <>
      {Array.from({ length: rows }, (_, i) => (
        <li key={i} className="px-4 py-4">
          <div className="h-4 w-2/3 animate-pulse rounded bg-gray-200" aria-hidden />
          <div className="mt-2 h-3 w-1/3 animate-pulse rounded bg-gray-100" aria-hidden />
        </li>
      ))}
    </>
  )
}

// Строка списка: слева заголовок и метаданные, справа бейдж статуса и ссылка-действие.
export function ListRow({
  title,
  meta,
  badge,
  actionTo,
  actionLabel,
}: {
  title: string
  meta: ReactNode
  badge?: ReactNode
  actionTo: string
  actionLabel: string
}) {
  return (
    <li className="flex flex-col gap-2 px-4 py-3 hover:bg-gray-50 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium text-gray-900">{title}</p>
        <p className="mt-0.5 text-sm text-gray-500">{meta}</p>
      </div>
      <div className="flex shrink-0 items-center gap-3">
        {badge}
        <Link to={actionTo} className="text-sm font-medium text-primary-600 hover:text-primary-700">
          {actionLabel}
        </Link>
      </div>
    </li>
  )
}

// Баннер проверки компании: черновик и отказ требуют действий, «на проверке» — только информирует.
// Причина отказа приходит из GET /organizations/me (в /auth/me этого поля нет).
export function KycBanner({ status, rejectReason }: { status: KycStatus; rejectReason?: string | null }) {
  const { t } = useTranslation()
  const Icon = status === 'verified' ? CheckCircle2 : status === 'under_review' ? Clock : AlertTriangle
  const tone =
    status === 'verified'
      ? 'border-success/30 bg-success/5 text-success'
      : status === 'rejected'
        ? 'border-danger/30 bg-danger/5 text-danger'
        : 'border-warning/30 bg-warning/5 text-warning'

  return (
    <div
      className={cn(
        'flex flex-col gap-3 rounded-md border p-4 sm:flex-row sm:items-center sm:justify-between',
        tone,
      )}
    >
      <div className="flex items-start gap-3">
        <Icon className="mt-0.5 h-5 w-5 shrink-0" aria-hidden />
        <div>
          <p className="text-sm font-semibold">
            {t('dashboard.kyc.bannerTitle', { status: t(`labels.kycStatus.${status}`) })}
          </p>
          <p className="mt-0.5 text-sm opacity-90">
            {status === 'draft' && t('dashboard.kyc.draftHint')}
            {status === 'under_review' && t('dashboard.kyc.reviewHint')}
            {status === 'rejected' && t('dashboard.kyc.rejectedHint', { reason: rejectReason ?? '' })}
            {status === 'verified' && t('dashboard.kyc.verifiedHint')}
          </p>
        </div>
      </div>
      {status !== 'verified' && (
        <Link
          to="/profile/kyc"
          className="inline-flex h-8 shrink-0 items-center justify-center rounded-md border border-current px-3 text-sm font-medium hover:bg-white/60"
        >
          {t('dashboard.kyc.cta')}
        </Link>
      )}
    </div>
  )
}

export function StatusBadge({ status }: { status: ApplicationStatus }) {
  return <Badge variant={APPLICATION_BADGE_VARIANT[status]}>{applicationStatusLabel(status)}</Badge>
}

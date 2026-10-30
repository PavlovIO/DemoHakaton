import { useTranslation } from 'react-i18next'
import { useAuth } from '@/hooks/useAuth'
import { BorrowerDashboard } from '@/components/dashboard/BorrowerDashboard'
import { LenderDashboard } from '@/components/dashboard/LenderDashboard'

export function Dashboard() {
  const { t } = useTranslation()
  const { user } = useAuth()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">{t('dashboard.title')}</h1>
        <p className="mt-2 text-gray-600">{t('dashboard.welcome')}</p>
      </div>
      {/* Дашборд разный по ролям; для администратора витрина не предусмотрена. */}
      {user?.role === 'borrower' && <BorrowerDashboard />}
      {user?.role === 'lender' && <LenderDashboard />}
      {user?.role === 'admin' && <p className="text-sm text-gray-500">{t('dashboard.adminHint')}</p>}
    </div>
  )
}

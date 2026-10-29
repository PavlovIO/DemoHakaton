import { NavLink, Navigate, Outlet } from 'react-router-dom'
import { useAdminQueueCounts } from './admin/Counts'

const TABS = [
  { path: '/admin/kyc', totalKey: 'kyc' as const, label: 'Проверка компаний' },
  { path: '/admin/applications', totalKey: 'moderation' as const, label: 'Модерация заявок' },
]

// Layout панели администратора: заголовок + вкладки-маршруты (пункт 10:
// «присутствующие вкладки разделить» — KYC и Модерация теперь отдельные страницы).
export function AdminPanel() {
  const { kycTotal, moderationTotal } = useAdminQueueCounts()
  const totals = { kyc: kycTotal, moderation: moderationTotal }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Панель администратора</h1>
        <p className="mt-1 text-sm text-gray-500">Проверка компаний и модерация заявок</p>
      </div>

      {/* Табы */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex space-x-8">
          {TABS.map(tab => (
            <NavLink
              key={tab.path}
              to={tab.path}
              className={({ isActive }) =>
                `whitespace-nowrap pb-4 px-1 border-b-2 font-medium text-sm ${
                  isActive
                    ? 'border-primary-500 text-primary-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                }`
              }
            >
              {`${tab.label} (${totals[tab.totalKey]})`}
            </NavLink>
          ))}
        </nav>
      </div>

      <Outlet />
    </div>
  )
}

// /admin без вложенного пути — на первую вкладку.
export function AdminIndexRedirect() {
  return <Navigate to="/admin/kyc" replace />
}
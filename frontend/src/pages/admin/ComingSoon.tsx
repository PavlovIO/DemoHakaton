// Заглушки под шаги 3C–3E: AML-флаги, Пользователи, Аналитика.
import { Link } from 'react-router-dom'

const LINKS = [
  { to: '/admin/kyc', label: 'Очередь KYC' },
  { to: '/admin/applications', label: 'Модерация заявок' },
]

function ComingSoon({ title, note }: { title: string; note?: string }) {
  return (
    <div className="bg-white shadow sm:rounded-md p-8 text-center space-y-3">
      <h2 className="text-lg font-semibold text-gray-900">{title}</h2>
      <p className="text-sm text-gray-500">{note ?? 'Раздел в разработке.'}</p>
      <div className="flex justify-center gap-4 pt-2">
        {LINKS.map(l => (
          <Link key={l.to} to={l.to} className="text-sm text-primary-600 hover:text-primary-700">
            {l.label}
          </Link>
        ))}
      </div>
    </div>
  )
}

export function AmlComingSoon() {
  return <ComingSoon title="AML-флаги" note="Вкладка AML-флагов появится в шаге 3C." />
}

export function UsersComingSoon() {
  return <ComingSoon title="Пользователи" note="Управление пользователями появится в шаге 3D." />
}

export function AnalyticsComingSoon() {
  return <ComingSoon title="Аналитика" note="Дашборд аналитики появится в шаге 3E." />
}

import { cn } from '@/lib/utils'
import { useAuth, type UserRole } from '@/hooks/useAuth'
import {
  LayoutDashboard,
  FileText,
  MessageSquare,
  Briefcase,
  CheckCircle,
  Users,
  AlertTriangle,
  BarChart3,
  ScrollText,
} from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { type AdminQueue, QueueCountBadge } from './QueueCountBadge'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'

interface NavItem {
  to: string
  label: string
  icon: React.ReactNode
  queue?: AdminQueue
}

// Функция, а не константа: подписи клиентов берутся из словаря на текущем языке.
const navigationConfig = (t: TFunction): Record<UserRole, NavItem[]> => ({
  borrower: [
    { to: '/dashboard', label: t('nav.dashboard'), icon: <LayoutDashboard className="h-4 w-4" /> },
    { to: '/applications', label: t('nav.applications'), icon: <FileText className="h-4 w-4" /> },
    // Вкладки «Предложения» у заёмщика нет: входящие предложения видны в карточке заявки.
    { to: '/deals', label: t('nav.deals'), icon: <Briefcase className="h-4 w-4" /> },
  ],
  lender: [
    { to: '/dashboard', label: t('nav.dashboard'), icon: <LayoutDashboard className="h-4 w-4" /> },
    { to: '/marketplace', label: t('nav.marketplace'), icon: <FileText className="h-4 w-4" /> },
    { to: '/offers', label: t('nav.myOffers'), icon: <MessageSquare className="h-4 w-4" /> },
    { to: '/deals', label: t('nav.deals'), icon: <Briefcase className="h-4 w-4" /> },
  ],
  admin: [
    { to: '/admin/analytics', label: 'Аналитика', icon: <BarChart3 className="h-4 w-4" /> },
    // Бейдж очереди есть только у админа: у других ролей запросов счётчиков нет.
    { to: '/admin/kyc', label: 'Проверка компаний', icon: <CheckCircle className="h-4 w-4" />, queue: 'kyc' },
    { to: '/admin/applications', label: 'Модерация заявок', icon: <FileText className="h-4 w-4" />, queue: 'moderation' },
    { to: '/admin/aml', label: 'AML-флаги', icon: <AlertTriangle className="h-4 w-4" /> },
    { to: '/admin/users', label: 'Пользователи', icon: <Users className="h-4 w-4" /> },
    { to: '/admin/audit', label: 'Аудит', icon: <ScrollText className="h-4 w-4" /> },
  ],
})

export function Navigation() {
  const { user } = useAuth()
  const { t } = useTranslation()

  if (!user) return null

  const navItems = navigationConfig(t)[user.role]

  return (
    // На мобильных навигация на втором уровне шапки: подписи видны, кнопки тянутся на всю ширину (flex-1).
    // На lg+ подписи скрыты (иконки + aria-label), чтобы не распирать одну строку с логотипом.
    <nav className="flex flex-wrap items-center gap-1">
      {navItems.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          // Подпись скрыта до lg — без явного имени ссылка читалась бы как одно число бейджа.
          aria-label={item.label}
          className={({ isActive }) =>
            cn(
              // shrink-0 + whitespace-nowrap: текст кнопок меню никогда не обрезается; если строк
              // больше трёх (админ), второй уровень шапки даёт горизонтальный скролл только внутри себя.
              'inline-flex min-h-10 shrink-0 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors',
              isActive
                ? 'bg-primary-50 text-primary-700'
                : 'text-gray-700 hover:bg-white/50 hover:text-gray-900'
            )
          }
        >
          {({ isActive }) => (
            <>
              <span className="relative">
                {item.icon}
                {item.queue && <QueueCountBadge queue={item.queue} onQueuePage={isActive} />}
              </span>
              {/* Подпись: на мобильных видна (второй уровень шапки), на lg показываем только иконки,
                  чтобы строка с логотипом не расползалась в горизонтальный скролл. whitespace-nowrap —
                  текст кнопки не режется по буквам при поджатии flex-элементами. */}
              <span className="whitespace-nowrap lg:hidden">{item.label}</span>
            </>
          )}
        </NavLink>
      ))}
    </nav>
  )
}
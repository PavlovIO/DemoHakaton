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
import { QueueCountBadge } from './QueueCountBadge'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'

interface NavItem {
  to: string
  label: string
  icon: React.ReactNode
  badge?: React.ReactNode
}

// Функция, а не константа: подписи клиентов берутся из словаря на текущем языке.
const navigationConfig = (t: TFunction): Record<UserRole, NavItem[]> => ({
  borrower: [
    { to: '/dashboard', label: t('nav.dashboard'), icon: <LayoutDashboard className="h-4 w-4" /> },
    { to: '/applications', label: t('nav.applications'), icon: <FileText className="h-4 w-4" /> },
    { to: '/offers', label: t('nav.offers'), icon: <MessageSquare className="h-4 w-4" /> },
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
    // Бейдж монтируется только у админа: у других ролей запросов счётчиков нет.
    { to: '/admin/kyc', label: 'Проверка компаний', icon: <CheckCircle className="h-4 w-4" />, badge: <QueueCountBadge queue="kyc" /> },
    { to: '/admin/applications', label: 'Модерация заявок', icon: <FileText className="h-4 w-4" />, badge: <QueueCountBadge queue="moderation" /> },
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
    <nav className="flex items-center gap-1">
      {navItems.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          className={({ isActive }) =>
            cn(
              'inline-flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors',
              isActive
                ? 'bg-primary-50 text-primary-700'
                : 'text-gray-700 hover:bg-white/50 hover:text-gray-900'
            )
          }
        >
          <span className="relative">
            {item.icon}
            {item.badge}
          </span>
          <span className="hidden lg:inline">{item.label}</span>
        </NavLink>
      ))}
    </nav>
  )
}
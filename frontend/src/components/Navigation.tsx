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

interface NavItem {
  to: string
  label: string
  icon: React.ReactNode
}

const navigationConfig: Record<UserRole, NavItem[]> = {
  borrower: [
    { to: '/dashboard', label: 'Дашборд', icon: <LayoutDashboard className="h-4 w-4" /> },
    { to: '/applications', label: 'Мои заявки', icon: <FileText className="h-4 w-4" /> },
    { to: '/offers', label: 'Предложения', icon: <MessageSquare className="h-4 w-4" /> },
    { to: '/deals', label: 'Сделки', icon: <Briefcase className="h-4 w-4" /> },
  ],
  lender: [
    { to: '/dashboard', label: 'Дашборд', icon: <LayoutDashboard className="h-4 w-4" /> },
    { to: '/marketplace', label: 'Витрина заявок', icon: <FileText className="h-4 w-4" /> },
    { to: '/offers', label: 'Мои предложения', icon: <MessageSquare className="h-4 w-4" /> },
    { to: '/deals', label: 'Сделки', icon: <Briefcase className="h-4 w-4" /> },
  ],
  admin: [
    { to: '/admin/analytics', label: 'Аналитика', icon: <BarChart3 className="h-4 w-4" /> },
    { to: '/admin/kyc', label: 'Очередь KYC', icon: <CheckCircle className="h-4 w-4" /> },
    { to: '/admin/applications', label: 'Модерация заявок', icon: <FileText className="h-4 w-4" /> },
    { to: '/admin/aml', label: 'AML-флаги', icon: <AlertTriangle className="h-4 w-4" /> },
    { to: '/admin/users', label: 'Пользователи', icon: <Users className="h-4 w-4" /> },
    { to: '/admin/audit', label: 'Аудит', icon: <ScrollText className="h-4 w-4" /> },
  ],
}

export function Navigation() {
  const { user } = useAuth()

  if (!user) return null

  const navItems = navigationConfig[user.role]

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
          {item.icon}
          <span className="hidden lg:inline">{item.label}</span>
        </NavLink>
      ))}
    </nav>
  )
}
import { Link, useLocation } from 'react-router-dom'
import { ChevronRight, Home } from 'lucide-react'

interface BreadcrumbItem {
  label: string
  href?: string
}

const routeLabels: Record<string, string> = {
  dashboard: 'Дашборд',
  applications: 'Заявки',
  offers: 'Предложения',
  deals: 'Сделки',
  kyc: 'Проверка компании',
  new: 'Создать',
  admin: 'Администрирование',
  organizations: 'Проверка компаний',
  aml: 'AML-флаги',
  users: 'Пользователи',
  audit: 'Аудит',
}

export function Breadcrumbs() {
  const location = useLocation()
  const pathnames = location.pathname.split('/').filter((x) => x)

  if (pathnames.length === 0) return null

  const breadcrumbs: BreadcrumbItem[] = [
    { label: 'Главная', href: '/' },
    ...pathnames.map((segment, index) => {
      const path = `/${pathnames.slice(0, index + 1).join('/')}`
      const isLast = index === pathnames.length - 1
      const label = routeLabels[segment] || segment

      return {
        label: isLast ? label : label,
        href: isLast ? undefined : path,
      }
    }),
  ]

  return (
    <nav className="flex items-center gap-1 text-sm text-gray-700">
      {breadcrumbs.map((breadcrumb, index) => (
        <div key={breadcrumb.href || breadcrumb.label} className="flex items-center gap-1">
          {index > 0 && <ChevronRight className="h-4 w-4" />}
          {breadcrumb.href ? (
            <Link to={breadcrumb.href} className="hover:text-gray-900">
              {index === 0 ? <Home className="h-4 w-4" /> : breadcrumb.label}
            </Link>
          ) : (
            <span className="font-medium text-gray-900">{breadcrumb.label}</span>
          )}
        </div>
      ))}
    </nav>
  )
}
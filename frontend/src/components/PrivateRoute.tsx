import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuth, type UserRole } from '@/hooks/useAuth'

interface Props {
  children: ReactNode
  requireKyc?: boolean
  requireRole?: UserRole
}

const KYC_REQUIRED_ROLES: readonly UserRole[] = ['borrower', 'lender']

export function PrivateRoute({ children, requireKyc = true, requireRole }: Props) {
  const { isLoading, isAuthenticated, user, organization } = useAuth()
  const location = useLocation()

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-gray-500">
        Загрузка…
      </div>
    )
  }

  if (!isAuthenticated || !user) {
    return (
      <Navigate  to="/login" state={{ from: location.pathname + location.search }} replace/>
    )
  }

  if (requireRole && user.role !== requireRole) {
    return <Navigate to="/dashboard" replace />
  }

  const kycRequired =
    requireKyc && KYC_REQUIRED_ROLES.includes(user.role)

  if (kycRequired && organization?.kyc_status !== 'verified') {
    if (location.pathname.startsWith('/profile')) {
      return <>{children}</>
    }
    const destination = organization ? '/profile/kyc' : '/profile'
    return <Navigate to={destination} replace />
  }

  return <>{children}</>
}
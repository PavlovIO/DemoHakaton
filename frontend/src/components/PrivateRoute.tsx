import { Navigate, useLocation } from 'react-router-dom'
import { useAuth, type UserRole } from '@/hooks/useAuth'
import { type ReactNode } from 'react'

interface PrivateRouteProps {
  children: ReactNode
  allowedRoles?: UserRole[]
  requireKyc?: boolean
}

export function PrivateRoute({ children, allowedRoles, requireKyc = false }: PrivateRouteProps) {
  const { user } = useAuth()
  const location = useLocation()

  if (!user) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  if (allowedRoles && !allowedRoles.includes(user.role)) {
    return <Navigate to="/dashboard" replace />
  }

  if (requireKyc && user.kycStatus !== 'verified') {
    return <Navigate to="/kyc" replace />
  }

  return <>{children}</>
}
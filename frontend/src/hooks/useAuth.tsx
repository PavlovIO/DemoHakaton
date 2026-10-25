import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'
import {
  useQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api-client'
import { handleApiError } from '@/lib/api-errors'
import type { components } from '@/lib/api-schema'

// Типы из OpenAPI
export type Me = components['schemas']['Me']
export type User = NonNullable<Me>['user']
export type Organization = NonNullable<NonNullable<Me>['organization']>
export type UserRole = User['role']
export type KycStatus = Organization['kyc_status']
export type RegisterPayload = components['schemas']['RegisterRequest']

interface AuthContextValue {
  me: Me | null
  user: User | null
  organization: Organization | null
  isLoading: boolean
  isAuthenticated: boolean
  isVerified: boolean
  needsOnboarding: boolean

  login: (email: string, password: string) => Promise<void>
  register: (payload: RegisterPayload) => Promise<void>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

const ME_KEY = ['me'] as const

const KYC_REQUIRED_ROLES: readonly UserRole[] = ['borrower', 'lender']

// Список статусов, при которых мы включаем мок-режим для разработки
// 404/500/501 - бэкенд не готов
// 502 Bad Gateway - Vite не может достучаться до бэкенда (он выключен)
const MOCK_FALLBACK_STATUSES = [404, 500, 501, 502]

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()

  const { data: me = null, isLoading } = useQuery<Me | null>({
    queryKey: ME_KEY,
    queryFn: async () => {
      try {
        return await api.get<Me>('/auth/me')
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null
        throw err
      }
    },
    staleTime: 60_000,
    retry: false,
    refetchOnWindowFocus: true,
  })

  const user = me?.user ?? null
  const organization = me?.organization ?? null

  const isAuthenticated = !!me && !!user

  const isVerified =
    organization !== null && organization.kyc_status === 'verified'

  const needsOnboarding =
    isAuthenticated &&
    !isVerified &&
    user !== null &&
    KYC_REQUIRED_ROLES.includes(user.role)

  // ==========================================
  // ЛОГИН
  // ==========================================
  const loginMutation = useMutation({
    mutationFn: async ({ email, password }: { email: string; password: string }) => {
      try {
        return await api.post<Me>('/auth/login', { email, password })
      } catch (err) {
        // ⚠️ FALLBACK ДЛЯ РАЗРАБОТКИ
        if (import.meta.env.DEV && err instanceof ApiError && MOCK_FALLBACK_STATUSES.includes(err.status)) {
          console.warn('⚠️ Бэкенд auth недоступен. Используем MOCK-пользователя для входа.')
          return getMockUser(email)
        }
        throw err
      }
    },
    onSuccess: (data) => {
      queryClient.setQueryData(ME_KEY, data)
    },
    onError: (err) => {
      if (!(import.meta.env.DEV && err instanceof ApiError && MOCK_FALLBACK_STATUSES.includes(err.status))) {
        handleApiError(err)
      }
    },
  })

  // ==========================================
  // РЕГИСТРАЦИЯ
  // ==========================================
  const registerMutation = useMutation({
    mutationFn: async (payload: RegisterPayload) => {
      try {
        return await api.post<Me>('/auth/register', payload)
      } catch (err) {
        // ⚠️ FALLBACK ДЛЯ РАЗРАБОТКИ
        if (import.meta.env.DEV && err instanceof ApiError && MOCK_FALLBACK_STATUSES.includes(err.status)) {
          console.warn('⚠️ Бэкенд auth недоступен. Используем MOCK-регистрацию.')
          // Передаем роль из формы (borrower/lender), чтобы мок создал правильную компанию
          return getMockUser(payload.email, payload.role)
        }
        throw err
      }
    },
    onSuccess: (data) => {
      queryClient.setQueryData(ME_KEY, data)
    },
    onError: (err) => {
      if (!(import.meta.env.DEV && err instanceof ApiError && MOCK_FALLBACK_STATUSES.includes(err.status))) {
        handleApiError(err)
      }
    },
  })

  // ==========================================
  // ВЫХОД
  // ==========================================
  const logoutMutation = useMutation({
    mutationFn: () => api.post<void>('/auth/logout'),
    onSettled: () => {
      queryClient.clear()
    },
    onError: handleApiError,
  })

  const login = useCallback(
    async (email: string, password: string) => {
      await loginMutation.mutateAsync({ email, password })
    },
    [loginMutation],
  )

  const register = useCallback(
    async (payload: RegisterPayload) => {
      await registerMutation.mutateAsync(payload)
    },
    [registerMutation],
  )

  const logout = useCallback(async () => {
    try {
      await logoutMutation.mutateAsync()
    } catch (err) {
      // Игнорируем ошибку logout в мок-режиме, чтобы просто выкинуть на логин
      if (import.meta.env.DEV) console.warn('Mock logout', err)
    }
    window.location.href = '/login'
  }, [logoutMutation])

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ME_KEY })
  }, [queryClient])

  const value = useMemo<AuthContextValue>(
    () => ({
      me, user, organization,
      isLoading,
      isAuthenticated,
      isVerified,
      needsOnboarding,
      login, register, logout, refresh,
    }),
    [me, user, organization, isLoading, isAuthenticated,
     isVerified, needsOnboarding, login, register, logout, refresh],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
/* eslint-disable react-refresh/only-export-components */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>')
  return ctx
}

// ==========================================
// ГЕНЕРАТОР МОКОВЫХ ДАННЫХ
// ==========================================
function getMockUser(email: string, forcedRole?: 'borrower' | 'lender' | 'admin'): Me {
  let role: 'borrower' | 'lender' | 'admin' = forcedRole || 'borrower'
  
  // Если роль не передали явно (например, при логине), угадываем по email
  if (!forcedRole) {
    if (email.includes('lender')) role = 'lender'
    if (email.includes('admin')) role = 'admin'
  }

  return {
    user: {
      id: 'mock-user-1',
      email: email,
      role: role,
    },
    organization: {
      id: 'mock-org-1',
      name: role === 'admin' ? 'Platform Admin' : (role === 'borrower' ? 'Рога и копыта' : 'Beta Corp'),
      country_code: role === 'borrower' ? 'RU' : 'CY',
      kyc_status: 'verified', // Сразу verified, чтобы не застревать на экране KYC при разработке
      legal_address: 'г. Москва, ул. Тверская, д. 1',
      registration_number: '1234567890',
    }
  } as unknown as Me
}
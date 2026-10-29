import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'
import { PAGE_SIZE, pagedList } from '@/lib/pagination'
import { COMPANY_CARD_KEY } from './useKyc'

const LIST_POLL_INTERVAL_MS = 30_000

type Schemas = components['schemas']
export type AdminUser = Schemas['User']
export type AmlFlag = Schemas['AmlFlag']
export type AnalyticsSummary = Schemas['AnalyticsSummary']

const ADMIN_ORGANIZATIONS_KEY = ['admin-organizations'] as const
const ADMIN_USERS_KEY = ['admin-users'] as const
const ADMIN_AML_FLAGS_KEY = ['admin-aml-flags'] as const
const ADMIN_ANALYTICS_KEY = ['admin-analytics'] as const

// --- Организации (справочник для фильтров AML и т.п.; KYC-очередь — useKycQueue) ---

export interface AdminOrganizationsParams {
  kyc_status?: Schemas['KycStatus'][]
  type?: Schemas['OrganizationType']
  country_code?: Schemas['CountryCode']
  q?: string
}

export function useAdminOrganizations(params: AdminOrganizationsParams = {}) {
  return useInfiniteQuery({
    queryKey: [...ADMIN_ORGANIZATIONS_KEY, params],
    queryFn: ({ pageParam }) =>
      api.get<Schemas['OrganizationPage']>('/admin/organizations', {
        query: { ...params, page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<Schemas['OrganizationPage']>(),
    refetchInterval: LIST_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  })
}

// --- Пользователи ---

export interface AdminUsersParams {
  role?: Schemas['Role']
  status?: Schemas['UserStatus']
  q?: string
}

export function useAdminUsers(params: AdminUsersParams = {}) {
  return useInfiniteQuery({
    queryKey: [...ADMIN_USERS_KEY, params],
    queryFn: ({ pageParam }) =>
      api.get<Schemas['UserPage']>('/admin/users', {
        query: { ...params, page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<Schemas['UserPage']>(),
    refetchInterval: LIST_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  })
}

// reason обязателен по контракту (AdminReason).
export function useBlockUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, reason }: { userId: string; reason: string }) =>
      api.post<AdminUser>(
        '/admin/users/{userId}/block',
        { reason },
        { path: { userId } },
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ADMIN_USERS_KEY }),
  })
}

export function useUnblockUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (userId: string) =>
      api.post<AdminUser>('/admin/users/{userId}/unblock', undefined, { path: { userId } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ADMIN_USERS_KEY }),
  })
}

// --- AML-флаги ---

export interface AmlFlagsParams {
  status?: Schemas['AmlFlagStatus']
  severity?: Schemas['AmlSeverity']
  organization_id?: string
  application_id?: string
}

export function useAmlFlags(params: AmlFlagsParams = {}) {
  return useInfiniteQuery({
    queryKey: [...ADMIN_AML_FLAGS_KEY, params],
    queryFn: ({ pageParam }) =>
      api.get<Schemas['AmlFlagPage']>('/admin/aml/flags', {
        query: { ...params, page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<Schemas['AmlFlagPage']>(),
    refetchInterval: LIST_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  })
}

export function useResolveAmlFlag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      flagId,
      resolution,
      comment,
    }: {
      flagId: number
      resolution: 'false_positive' | 'confirmed'
      comment: string
    }) =>
      api.post<AmlFlag>(
        '/admin/aml/flags/{flagId}/resolve',
        { resolution, comment },
        // flagId по контракту int64, путь подставляет строки.
        { path: { flagId: String(flagId) } },
      ),
    // Карточка компании показывает флаги, а открытый high блокирует подтверждение.
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ADMIN_AML_FLAGS_KEY }),
        queryClient.invalidateQueries({ queryKey: COMPANY_CARD_KEY }),
      ]),
  })
}

// --- Аналитика ---

export interface AnalyticsParams {
  created_from?: string
  created_to?: string
  top_n?: number
}

export function useAnalyticsSummary(params: AnalyticsParams = {}) {
  return useQuery({
    queryKey: [...ADMIN_ANALYTICS_KEY, params],
    queryFn: () => api.get<AnalyticsSummary>('/admin/analytics/summary', { query: { ...params } }),
    // Сводка дорогая: обновляем раз в минуту, чаще поллить не нужно.
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  })
}

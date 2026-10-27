import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components, operations } from '@/lib/api-schema'
import { PAGE_SIZE, pagedList } from '@/lib/pagination'

export type Organization = components['schemas']['Organization']
export type User = components['schemas']['User']
export type AmlFlag = components['schemas']['AmlFlag']
export type AnalyticsSummary = components['schemas']['AnalyticsSummary']
type OrganizationPage = components['schemas']['OrganizationPage']
type UserPage = components['schemas']['UserPage']
type AmlFlagPage = components['schemas']['AmlFlagPage']

export const ADMIN_ORGANIZATIONS_KEY = ['admin', 'organizations'] as const
export const ADMIN_USERS_KEY = ['admin', 'users'] as const
export const ADMIN_AML_FLAGS_KEY = ['admin', 'aml-flags'] as const
export const ADMIN_ANALYTICS_KEY = ['admin', 'analytics'] as const

const ANALYTICS_STALE_TIME_MS = 60_000

type OrgsQuery = NonNullable<operations['adminListOrganizations']['parameters']['query']>
type UsersQuery = NonNullable<operations['adminListUsers']['parameters']['query']>
type FlagsQuery = NonNullable<operations['adminListAmlFlags']['parameters']['query']>

// Компании; очередь верификации — kyc_status: ['under_review'].
export function useAdminOrganizations(params: Omit<OrgsQuery, 'page' | 'page_size'> = {}) {
  return useInfiniteQuery({
    queryKey: [...ADMIN_ORGANIZATIONS_KEY, params],
    queryFn: ({ pageParam }) =>
      api.get<OrganizationPage>('/admin/organizations', {
        query: { ...params, page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<OrganizationPage>(),
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

export function useAdminUsers(params: Omit<UsersQuery, 'page' | 'page_size'> = {}) {
  return useInfiniteQuery({
    queryKey: [...ADMIN_USERS_KEY, params],
    queryFn: ({ pageParam }) =>
      api.get<UserPage>('/admin/users', {
        query: { ...params, page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<UserPage>(),
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

export function useBlockUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, reason }: { userId: string; reason?: string }) =>
      api.post<User>('/admin/users/{userId}/block', { reason }, { path: { userId } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ADMIN_USERS_KEY }),
  })
}

export function useUnblockUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (userId: string) =>
      api.post<User>('/admin/users/{userId}/unblock', undefined, { path: { userId } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ADMIN_USERS_KEY }),
  })
}

export function useAmlFlags(params: Omit<FlagsQuery, 'page' | 'page_size'> = {}) {
  return useInfiniteQuery({
    queryKey: [...ADMIN_AML_FLAGS_KEY, params],
    queryFn: ({ pageParam }) =>
      api.get<AmlFlagPage>('/admin/aml/flags', {
        query: { ...params, page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<AmlFlagPage>(),
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

export type AmlResolution = 'false_positive' | 'confirmed'

export function useResolveAmlFlag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ flagId, resolution, comment }: { flagId: number; resolution: 'false_positive' | 'confirmed'; comment: string }) =>
      api.post<AmlFlag>('/admin/aml/flags/{flagId}/resolve', { resolution, comment }, { path: { flagId: String(flagId) } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ADMIN_AML_FLAGS_KEY })
      queryClient.invalidateQueries({ queryKey: ADMIN_ORGANIZATIONS_KEY })
    },
  })
}

export function useAnalyticsSummary(topN?: number) {
  return useQuery({
    queryKey: [...ADMIN_ANALYTICS_KEY, 'summary', topN ?? 10],
    queryFn: () => api.get<AnalyticsSummary>('/admin/analytics/summary', { query: { top_n: topN } }),
    staleTime: ANALYTICS_STALE_TIME_MS,
  })
}
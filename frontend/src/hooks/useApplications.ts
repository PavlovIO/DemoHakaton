import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components, operations } from '@/lib/api-schema'
import { PAGE_SIZE, pagedList } from '@/lib/pagination'

export type Application = components['schemas']['Application']
export type ApplicationCreate = components['schemas']['ApplicationCreate']
export type ApplicationUpdate = components['schemas']['ApplicationUpdate']
type ApplicationPage = components['schemas']['ApplicationPage']

export const APPLICATIONS_KEY = ['applications'] as const

// Заёмщику бэкенд отдаёт только заявки его организации.
export function useMyApplications() {
  return useInfiniteQuery({
    queryKey: [...APPLICATIONS_KEY, 'mine'],
    queryFn: ({ pageParam }) =>
      api.get<ApplicationPage>('/applications', { query: { page: pageParam, page_size: PAGE_SIZE } }),
    ...pagedList<ApplicationPage>(),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

type ListApplicationsQuery = NonNullable<operations['listApplications']['parameters']['query']>

// Бэкенд применяет amount_min только вместе с currency, без неё молча игнорирует.
export type MarketplaceFilters = Pick<ListApplicationsQuery, 'target_country'> &
  (Pick<ListApplicationsQuery, 'currency' | 'amount_min'> & { currency: string } | { currency?: undefined; amount_min?: never })

// Без status кредитору приходят и заявки с его офферами в любом статусе (архив откликов).
export function useMarketplace(filters: MarketplaceFilters = {}) {
  return useInfiniteQuery({
    queryKey: [...APPLICATIONS_KEY, 'marketplace', filters],
    queryFn: ({ pageParam }) =>
      api.get<ApplicationPage>('/applications', {
        query: { ...filters, status: ['published'], sort: 'created_at_desc', page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<ApplicationPage>(),
    // Пока грузится новый фильтр, показываем прежний список вместо «ничего не найдено».
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

const MODERATION_FILTER = { status: ['moderation'] }

export function useModerationQueue() {
  return useInfiniteQuery({
    queryKey: [...APPLICATIONS_KEY, 'moderation'],
    queryFn: ({ pageParam }) =>
      api.get<ApplicationPage>('/applications', {
        query: { ...MODERATION_FILTER, sort: 'created_at_desc', page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<ApplicationPage>(),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

// Размер очереди модерации: total считает бэкенд. Ключ под APPLICATIONS_KEY —
// мутации заявок сбрасывают и его.
export function useModerationQueueCount() {
  return useQuery({
    queryKey: [...APPLICATIONS_KEY, 'moderation', 'count'],
    queryFn: () => api.get<ApplicationPage>('/applications', { query: { ...MODERATION_FILTER, page_size: 1 } }),
    select: (page) => page.total,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

export function useApproveApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api.post<Application>('/admin/applications/{applicationId}/approve', undefined, { path: { applicationId: id } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

export function useRejectApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      api.post<Application>('/admin/applications/{applicationId}/reject', { reason }, { path: { applicationId: id } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

export function useCreateApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (payload: ApplicationCreate) => api.post<Application>('/applications', payload),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

export function useUpdateApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: ApplicationUpdate }) =>
      api.patch<Application>('/applications/{applicationId}', payload, { path: { applicationId: id } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

export function useSubmitApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api.post<Application>('/applications/{applicationId}/submit', undefined, { path: { applicationId: id } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'

export type Application = components['schemas']['Application']
export type ApplicationCreate = components['schemas']['ApplicationCreate']
type ApplicationPage = components['schemas']['ApplicationPage']

export const APPLICATIONS_KEY = ['applications'] as const

// Пагинации в UI пока нет — берём максимум страницы API.
const PAGE_SIZE = 100

// Заёмщику бэкенд отдаёт только заявки его организации.
export function useMyApplications() {
  return useQuery({
    queryKey: [...APPLICATIONS_KEY, 'mine'],
    queryFn: () => api.get<ApplicationPage>('/applications', { query: { page_size: PAGE_SIZE } }),
  })
}

export interface MarketplaceFilters {
  target_country?: string
  currency?: string
  amount_min?: string
}

// Кредитору бэкенд отдаёт только опубликованные заявки.
export function useMarketplace(filters: MarketplaceFilters = {}) {
  return useQuery({
    queryKey: [...APPLICATIONS_KEY, 'marketplace', filters],
    queryFn: () =>
      api.get<ApplicationPage>('/applications', {
        query: { ...filters, sort: 'created_at_desc', page_size: PAGE_SIZE },
      }),
    // Пока грузится новый фильтр, показываем прежний список вместо «ничего не найдено».
    placeholderData: keepPreviousData,
  })
}

export function useModerationQueue() {
  return useQuery({
    queryKey: [...APPLICATIONS_KEY, 'moderation'],
    queryFn: () =>
      api.get<ApplicationPage>('/applications', {
        query: { status: ['moderation'], sort: 'created_at_desc', page_size: PAGE_SIZE },
      }),
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

export function useSubmitApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api.post<Application>('/applications/{applicationId}/submit', undefined, { path: { applicationId: id } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

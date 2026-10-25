import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'

export type Application = components['schemas']['Application']
export type ApplicationCreate = components['schemas']['ApplicationCreate']
type ApplicationPage = components['schemas']['ApplicationPage']

export const APPLICATIONS_KEY = ['applications'] as const

// Заёмщику бэкенд отдаёт только заявки его организации.
export function useMyApplications() {
  return useQuery({
    queryKey: [...APPLICATIONS_KEY, 'mine'],
    queryFn: () => api.get<ApplicationPage>('/applications'),
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
        query: { ...filters, sort: 'created_at_desc' },
      }),
  })
}

export function useModerationQueue() {
  return useQuery({
    queryKey: [...APPLICATIONS_KEY, 'moderation'],
    queryFn: () =>
      api.get<ApplicationPage>('/applications', {
        query: { status: ['moderation'], sort: 'created_at_desc' },
      }),
  })
}

export function useApproveApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api.post<Application>(`/admin/applications/${id}/approve` as '/admin/applications/{applicationId}/approve'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

export function useRejectApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      api.post<Application>(`/admin/applications/${id}/reject` as '/admin/applications/{applicationId}/reject', { reason }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

// Создаёт черновик и сразу отправляет его на модерацию — как кнопка в форме.
export function useCreateApplication() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: ApplicationCreate) => {
      const draft = await api.post<Application>('/applications', payload)
      return api.post<Application>(`/applications/${draft.id}/submit` as '/applications/{applicationId}/submit')
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY }),
  })
}

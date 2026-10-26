import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'

export type Notification = components['schemas']['Notification']
type NotificationPage = components['schemas']['NotificationPage']

export const NOTIFICATIONS_KEY = ['notifications'] as const

// Уведомления создаёт бэкенд по событиям сделки — опрашиваем для колокольчика.
export function useNotifications() {
  return useQuery({
    queryKey: NOTIFICATIONS_KEY,
    queryFn: () => api.get<NotificationPage>('/notifications'),
    refetchInterval: 30_000,
  })
}

export function useMarkAllNotificationsRead() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<void>('/notifications/read-all'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: NOTIFICATIONS_KEY }),
  })
}

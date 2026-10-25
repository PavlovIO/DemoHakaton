import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'

// text — plain text: рендерить только как текст, никогда как HTML.
export type Message = components['schemas']['Message']

const messagesKey = (offerId: string) => ['offers', offerId, 'messages'] as const

// Сообщения собеседника приходят без push — опрашиваем, пока чат открыт.
export function useMessages(offerId: string | undefined) {
  return useQuery({
    queryKey: messagesKey(offerId ?? ''),
    queryFn: () => api.get<Message[]>(`/offers/${offerId}/messages` as '/offers/{offerId}/messages'),
    enabled: !!offerId,
    refetchInterval: 5_000,
  })
}

export function useSendMessage(offerId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (text: string) =>
      api.post<Message>(`/offers/${offerId}/messages` as '/offers/{offerId}/messages', { text }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: messagesKey(offerId) }),
  })
}

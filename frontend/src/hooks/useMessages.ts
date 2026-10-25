import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'

// text — plain text: рендерить только как текст, никогда как HTML.
export type Message = components['schemas']['Message']

// Не под ['offers']: инвалидация офферов не должна перезапрашивать все открытые чаты.
const messagesKey = (offerId: string) => ['messages', offerId] as const

// Максимум limit по API; бэкенд отдаёт сообщения по возрастанию id.
const BATCH = 200

// Вся история чата: листаем курсором after_id, пока не придёт неполная пачка.
async function fetchAllMessages(offerId: string) {
  const all: Message[] = []
  for (;;) {
    const batch = await api.get<Message[]>(`/offers/${offerId}/messages` as '/offers/{offerId}/messages', {
      query: { after_id: all.at(-1)?.id, limit: BATCH },
    })
    all.push(...batch)
    if (batch.length < BATCH) return all
  }
}

// Сообщения собеседника приходят без push — опрашиваем, пока чат открыт.
export function useMessages(offerId: string | undefined) {
  return useQuery({
    queryKey: messagesKey(offerId ?? ''),
    queryFn: () => fetchAllMessages(offerId!),
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

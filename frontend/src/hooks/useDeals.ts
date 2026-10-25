import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'

export type Deal = components['schemas']['Deal']
type DealPage = components['schemas']['DealPage']

export const DEALS_KEY = ['deals'] as const

// Бэкенд отдаёт сделки, где организация пользователя — заёмщик или кредитор.
export function useDeals() {
  return useQuery({
    queryKey: DEALS_KEY,
    // Пагинации в UI пока нет — берём максимум страницы API.
    queryFn: () => api.get<DealPage>('/deals', { query: { page_size: 100 } }),
  })
}

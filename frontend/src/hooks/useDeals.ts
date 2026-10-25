import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'
import { PAGE_SIZE, pagedList } from '@/lib/pagination'

export type Deal = components['schemas']['Deal']
type DealPage = components['schemas']['DealPage']

export const DEALS_KEY = ['deals'] as const

// Бэкенд отдаёт сделки, где организация пользователя — заёмщик или кредитор.
export function useDeals() {
  return useInfiniteQuery({
    queryKey: DEALS_KEY,
    queryFn: ({ pageParam }) => api.get<DealPage>('/deals', { query: { page: pageParam, page_size: PAGE_SIZE } }),
    ...pagedList<DealPage>(),
  })
}

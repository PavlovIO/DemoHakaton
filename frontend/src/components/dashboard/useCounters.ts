import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'
import { OBSERVE_ONLY } from '@/lib/pagination'

type ApplicationPage = components['schemas']['ApplicationPage']
type OfferPage = components['schemas']['OfferPage']

// Счётчик KPI: бэкенд считает total по всем записям, а не только загруженным страницам.
// Запрос минимальный (page_size=1) и только наблюдает за кэшем — опрашивает страница списка.
export function useListTotal<T extends ApplicationPage | OfferPage>(
  key: readonly unknown[],
  path: string,
  query?: Record<string, unknown>,
) {
  return useQuery<T>({
    queryKey: [...key, 'count', query ?? {}],
    queryFn: () => api.get<T>(path, { query: { ...query, page: 1, page_size: 1 } }),
    ...OBSERVE_ONLY,
  })
}

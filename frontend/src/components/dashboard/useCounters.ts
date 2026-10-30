import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components, paths } from '@/lib/api-schema'
import { OBSERVE_ONLY } from '@/lib/pagination'

type ListPage = components['schemas'][Extract<keyof components['schemas'], `${string}Page`>]
type ItemOf<P> = P extends { items: (infer T)[] } ? T : never

// Счётчик KPI: бэкенд считает total по всем записям, а не только загруженным страницам.
// Запрос минимальный (page_size=1) и только наблюдает за кэшем — опрашивает страница списка.
export function useListTotal<P extends ListPage>(
  key: readonly unknown[],
  path: keyof paths,
  query?: Record<string, unknown>,
) {
  return useQuery<{ total: number; firstItem?: ItemOf<P> }>({
    queryKey: [...key, 'count', query ?? {}],
    queryFn: async () => {
      const page = await api.get<P>(path, { query: { ...query, page: 1, page_size: 1 } })
      // TS не сужает p.items у обобщённого P до массива элементов.
      return { total: page.total, firstItem: (page.items as unknown[])[0] as ItemOf<P> | undefined }
    },
    ...OBSERVE_ONLY,
  })
}

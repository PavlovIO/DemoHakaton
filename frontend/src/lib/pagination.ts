import type { InfiniteData } from '@tanstack/react-query'
import type { components } from '@/lib/api-schema'

// Размер страницы списков; следующая догружается кнопкой «Показать ещё».
export const PAGE_SIZE = 20

type Schemas = components['schemas']
// Любая страница списка из контракта: ApplicationPage, OfferPage, DealPage...
type ListPage = Schemas[Extract<keyof Schemas, `${string}Page`>]
type ItemOf<P> = P extends { items: (infer T)[] } ? T : never

// Наблюдатель, который только читает кэш списка, а опрашивает его другой (страница списка):
// без запроса на монтировании, по таймеру и при фокусе окна. Пустой кэш всё равно загрузит.
export const OBSERVE_ONLY = {
  refetchOnMount: false,
  refetchOnWindowFocus: false,
  refetchInterval: false,
} as const

// Общие опции useInfiniteQuery для списков API c page/page_size. select склеивает
// загруженные страницы, и экран читает data.items / data.total как у обычного запроса.
export function pagedList<P extends ListPage>() {
  return {
    initialPageParam: 1,
    getNextPageParam: (last: P) => (last.page * last.page_size < last.total ? last.page + 1 : undefined),
    select: (data: InfiniteData<P, number>) => ({
      // TS не сужает p.items у обобщённого P до массива элементов.
      items: data.pages.flatMap((p) => p.items as ItemOf<P>[]),
      total: data.pages[0]?.total ?? 0,
    }),
  }
}

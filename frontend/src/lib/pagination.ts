import type { InfiniteData } from '@tanstack/react-query'

// Размер страницы списков; следующая догружается кнопкой «Показать ещё».
export const PAGE_SIZE = 20

interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

// Общие опции useInfiniteQuery для списков API c page/page_size. select склеивает
// загруженные страницы, и экран читает data.items / data.total как у обычного запроса.
export function pagedList<T>() {
  return {
    initialPageParam: 1,
    getNextPageParam: (last: Page<T>) => (last.page * last.page_size < last.total ? last.page + 1 : undefined),
    select: (data: InfiniteData<Page<T>, number>) => ({
      items: data.pages.flatMap((p) => p.items),
      total: data.pages[0]?.total ?? 0,
    }),
  }
}

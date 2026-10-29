import { useTranslation } from 'react-i18next'
import { Button } from './Button'

interface LoadMoreProps {
  query: {
    hasNextPage: boolean
    isFetchingNextPage: boolean
    fetchNextPage: () => unknown
  }
}

// Кнопка под списком из useInfiniteQuery; скрыта, когда страниц больше нет.
export function LoadMore({ query }: LoadMoreProps) {
  const { t } = useTranslation()
  if (!query.hasNextPage) return null
  return (
    <div className="flex justify-center p-4">
      <Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => query.fetchNextPage()}>
        {query.isFetchingNextPage ? t('common.loading') : t('common.loadMore')}
      </Button>
    </div>
  )
}

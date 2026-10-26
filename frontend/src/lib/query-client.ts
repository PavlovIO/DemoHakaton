import { QueryCache, QueryClient } from '@tanstack/react-query'
import { handleApiError } from './api-errors'

export const queryClient = new QueryClient({
  // Ошибка загрузки списка не должна выглядеть как пустой список.
  // Если данные уже есть, фоновые перезапросы (опрос чата) молчат.
  queryCache: new QueryCache({
    onError: (err, query) => {
      if (query.state.data === undefined) handleApiError(err)
    },
  }),
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: true, // данные обновляются при возврате на вкладку браузера без перезагрузки страницы
    },
    mutations: {
      retry: 0,
    },
  },
})

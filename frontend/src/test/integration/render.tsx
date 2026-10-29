import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { AuthProvider } from '@/hooks/useAuth'

// Провайдеры как в main.tsx, но свой QueryClient на тест — кэш /auth/me не протекает.
// initialEntries — стартовый адрес, если тест ходит по маршрутам.
export function AppProviders({ children, initialEntries }: { children: ReactNode; initialEntries?: string[] }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <AuthProvider>{children}</AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import '@/i18n'
import { queryClient } from '@/lib/query-client'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import App from './App'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
      <Toaster position="top-right" richColors closeButton />
    </QueryClientProvider>
  </StrictMode>,
)
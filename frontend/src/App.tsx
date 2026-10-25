import { AuthProvider } from '@/hooks/useAuth'
import { AppRouter } from '@/router'
import { useEffect } from 'react'
import { api } from '@/lib/api-client'

function App() {
  useEffect(() => {
    api.get('/healthz')
      .then((r) => console.log('[api] healthz OK:', r))
      .catch((e) => console.error('[api] healthz FAIL:', e))
  }, [])

  return (
    <AuthProvider>
      <AppRouter />
    </AuthProvider>
  )
}

export default App

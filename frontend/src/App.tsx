import { AuthProvider, useAuth } from '@/hooks/useAuth'
import { AppRouter } from '@/router'
import { useEffect } from 'react'
import { api } from '@/lib/api-client'
import { useRoleLanguage } from '@/i18n'

function RoleLanguage() {
  const { user } = useAuth()
  useRoleLanguage(user?.role)
  return null
}

function App() {
  useEffect(() => {
    api.get('/healthz')
      .then((r) => console.log('[api] healthz OK:', r))
      .catch((e) => console.error('[api] healthz FAIL:', e))
  }, [])

  return (
    <AuthProvider>
      <RoleLanguage />
      <AppRouter />
    </AuthProvider>
  )
}

export default App

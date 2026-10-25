import { describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useAuth } from './useAuth'
import { AppProviders } from '@/test/integration/render'

const PASSWORD = 'Demo-Pass-2026'

async function renderAuth() {
  const hook = renderHook(() => useAuth(), { wrapper: AppProviders })
  await waitFor(() => expect(hook.result.current.isLoading).toBe(false))
  return hook
}

describe('useAuth против живого бэкенда', () => {
  it('гость: без сессии не авторизован', async () => {
    const { result } = await renderAuth()

    expect(result.current.isAuthenticated).toBe(false)
    expect(result.current.me).toBeNull()
  })

  it('верифицированный заёмщик: авторизован, KYC пройден, онбординг не нужен', async () => {
    const { result } = await renderAuth()

    await act(() => result.current.login('alpha@liquidity.local', PASSWORD))

    await waitFor(() => expect(result.current).toMatchObject({
      isAuthenticated: true,
      isVerified: true,
      needsOnboarding: false,
      user: { email: 'alpha@liquidity.local', role: 'borrower' },
      organization: { kyc_status: 'verified' },
    }))
  })

  it('админ без организации: авторизован, онбординг не нужен', async () => {
    const { result } = await renderAuth()

    await act(() => result.current.login('admin@liquidity.local', PASSWORD))

    await waitFor(() => expect(result.current).toMatchObject({
      isAuthenticated: true,
      isVerified: false,
      needsOnboarding: false,
      user: { role: 'admin' },
      organization: null,
    }))
  })

  it('выход: бэкенд завершил сессию', async () => {
    const { result } = await renderAuth()
    await act(() => result.current.login('alpha@liquidity.local', PASSWORD))

    await act(() => result.current.logout())

    // После logout приложение перезагружается на /login, поэтому смотрим, что увидит
    // новая загрузка в том же «браузере»: cookie остались, сессии на бэкенде нет.
    const { result: fresh } = await renderAuth()
    expect(fresh.current.isAuthenticated).toBe(false)
  })
})

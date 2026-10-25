import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { act, fireEvent, renderHook, screen } from '@testing-library/react'
import { ProfileDropdown } from './ProfileDropdown'
import { useAuth } from '@/hooks/useAuth'
import { AppProviders } from '@/test/integration/render'

function WithDropdown({ children }: { children: ReactNode }) {
  return (
    <AppProviders>
      {children}
      <ProfileDropdown />
    </AppProviders>
  )
}

async function renderLoggedIn(email: string) {
  const { result } = renderHook(() => useAuth(), { wrapper: WithDropdown })
  await act(() => result.current.login(email, 'Demo-Pass-2026'))
  fireEvent.click(await screen.findByRole('button', { expanded: false }))
}

describe('ProfileDropdown против живого бэкенда', () => {
  it('админ без организации: меню показывает почту и роль', async () => {
    await renderLoggedIn('admin@liquidity.local')

    const menu = await screen.findByRole('menu')
    expect(menu.textContent).toContain('admin@liquidity.local')
    expect(menu.textContent).toContain('Администратор')
  })

  it('заёмщик: меню показывает компанию и статус KYC', async () => {
    await renderLoggedIn('vector@liquidity.local')

    const menu = await screen.findByRole('menu')
    expect(menu.textContent).toContain('Вектор')
    expect(menu.textContent).toContain('Заёмщик')
    expect(menu.textContent).toContain('Подтверждено')
  })
})

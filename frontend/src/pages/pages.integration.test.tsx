import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { Offers } from './Offers'
import { Marketplace } from './Marketplace'
import { KycStub } from './KycStub'
import { PrivateRoute } from '@/components/PrivateRoute'
import { AppProviders } from '@/test/integration/render'
import { ADMIN, BORROWER, loginAs } from '@/test/integration/actors'

let fetchSpy: MockInstance<typeof fetch>
beforeEach(() => {
  fetchSpy = vi.spyOn(globalThis, 'fetch')
})
afterEach(() => fetchSpy.mockRestore())

// Как в router.tsx: страница рендерится, только когда /auth/me ответил.
function renderPage(page: ReactElement, opts: { requireKyc?: boolean } = {}) {
  return render(<PrivateRoute requireKyc={opts.requireKyc}>{page}</PrivateRoute>, { wrapper: AppProviders })
}

// Пути API, которые страница запросила за время теста.
function requestedPaths() {
  return fetchSpy.mock.calls.map(([input]) => new URL(String(input), 'http://localhost').pathname)
}

describe('страницы: чужая роль не запрашивает чужие данные', () => {
  it('заёмщик на «Моих офферах» видит подсказку и не запрашивает офферы', async () => {
    loginAs(BORROWER)
    renderPage(<Offers />)

    await screen.findByText('Входящие предложения отображаются в карточке заявки.')
    expect(requestedPaths()).not.toContain('/api/v1/offers')
  })

  it('заёмщик на витрине видит отказ и не запрашивает заявки', async () => {
    loginAs(BORROWER)
    renderPage(<Marketplace />)

    await screen.findByText('Доступно только для кредиторов.')
    expect(requestedPaths()).not.toContain('/api/v1/applications')
  })
})

describe('KYC без компании', () => {
  it('учётка без организации получает ссылку на профиль, а не вечную загрузку', async () => {
    loginAs(ADMIN)
    renderPage(<KycStub />, { requireKyc: false })

    const link = await screen.findByRole('link', { name: 'Заполнить профиль' })
    expect(link.getAttribute('href')).toBe('/profile')
  })
})

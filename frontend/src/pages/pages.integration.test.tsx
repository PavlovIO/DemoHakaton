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

// Черновик: правка допустима ровно в статусах draft/rejected — тех, что допускает бэк.
async function createDraft(): Promise<Application> {
  loginAs(BORROWER)
  return api.post<Application>('/applications', {
    amount: '120000.00',
    currency: 'EUR',
    target_country_code: 'DE',
    term_months: 6,
    transfer_method: 'sepa',
    purpose: `Заявка для редактирования ${crypto.randomUUID()}`,
  })
}

function requestBodyOf(method: 'PATCH' | 'POST'): unknown {
  const call = fetchSpy.mock.calls.find(
    ([input, init]) => new URL(String(input), 'http://localhost').pathname === '/api/v1/applications' && init?.method === method,
  )
  return call ? JSON.parse(String(call[1]?.body)) : undefined
}

describe('редактирование заявки (шаг 2C)', () => {
  it('кнопка «Редактировать» есть у черновика и открывает предзаполненную модалку; PATCH уходит с новым телом', async () => {
    const app = await createDraft()
    const newPurpose = `${app.purpose} — исправлено`

    renderPage(<Applications />)
    await screen.findByText(app.purpose)

    // Черновик можно править и подать; кнопки ровно для статусов, что допускает бэк.
    const editButton = screen.getByRole('button', { name: 'Редактировать' })
    expect(screen.getByRole('button', { name: 'Отправить на модерацию' })).toBeTruthy()

    fireEvent.click(editButton)

    // Та же модалка, что и при создании, но в режиме правки и с предзаполненными полями.
    await screen.findByText('Редактирование заявки')
    const amountInput = screen.getByPlaceholderText('500000.00') as HTMLInputElement
    expect(amountInput.value).toBe(app.amount)
    const purposeField = screen.getByDisplayValue(app.purpose) as HTMLTextAreaElement
    fireEvent.change(purposeField, { target: { value: newPurpose } })
    fireEvent.change(screen.getByDisplayValue('6') as HTMLInputElement, { target: { value: '9' } })

    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))

    await waitFor(() => expect(requestBodyOf('PATCH')).toEqual({
      amount: '120000.00',
      currency: 'EUR',
      target_country_code: 'DE',
      transfer_method: 'sepa',
      purpose: newPurpose,
      term_months: 9,
    }))

    // Статус не изменился (сохранение — только PATCH), список обновился инвалидацией.
    await screen.findByText(newPurpose)
    const updated = await api.get<Application>('/applications/{applicationId}', { path: { applicationId: app.id } })
    expect(updated).toMatchObject({ status: 'draft', purpose: newPurpose, term_months: 9 })
  })

  it('у опубликованной заявки кнопок «Редактировать» и «Отправить на модерацию» нет', async () => {
    const published = await publishApplication()
    loginAs(BORROWER)

    renderPage(<Applications />)
    await screen.findByText(published.purpose)

    expect(screen.queryByText('Редактирование заявки')).toBeNull()
    for (const button of screen.getAllByRole('button', { name: 'Редактировать' })) {
      // Карточка этой заявки не должна предлагать правку вовсе.
      expect(button.closest('li')?.textContent).not.toContain(published.number)
    }
    expect(screen.getAllByRole('button', { name: 'Отправить на модерацию' }).every((b) => !b.closest('li')?.textContent?.includes(published.number))).toBe(true)
  })
})

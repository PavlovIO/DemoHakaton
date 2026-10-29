import { beforeAll, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { Navigation } from '@/components/Navigation'
import { PrivateRoute } from '@/components/PrivateRoute'
import { KycStub } from '@/pages/KycStub'
import { AppProviders } from '@/test/integration/render'
import { ADMIN, BORROWER, loginAs } from '@/test/integration/actors'
import { kycSubmittedApplicant, submitApplication } from '@/test/integration/fixtures'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'

type Schemas = components['schemas']

// Одна компания в очереди проверки на весь файл: регистраций 10 в час с IP.
let applicant: Awaited<ReturnType<typeof kycSubmittedApplicant>>
beforeAll(async () => {
  applicant = await kycSubmittedApplicant()
  await submitApplication()
})

// Ссылка ведёт на настоящий файл: GET с cookie сессии текущего актёра.
async function expectDownloadable(link: HTMLElement) {
  const res = await fetch(link.getAttribute('href')!)
  expect(res.status).toBe(200)
}

describe('компания на проверке видит свои документы', () => {
  it('после отправки на проверку документы видны и скачиваются', async () => {
    applicant.resume()
    render(
      <PrivateRoute requireKyc={false}>
        <KycStub />
      </PrivateRoute>,
      { wrapper: AppProviders },
    )

    await screen.findByText('Документы на проверке')
    const charter = await screen.findByRole('link', { name: 'charter.pdf' })
    expect(screen.getByRole('link', { name: 'address_proof.pdf' })).toBeTruthy()
    expect(screen.queryByText('Загрузить файл')).toBeNull()
    await expectDownloadable(charter)
  })
})

describe('меню админа: размер очередей', () => {
  it('у «Проверки компаний» и «Модерации заявок» — total очередей с бэкенда', async () => {
    loginAs(ADMIN)
    const kyc = await api.get<Schemas['OrganizationPage']>('/admin/organizations', {
      query: { kyc_status: ['under_review'], page_size: 1 },
    })
    const moderation = await api.get<Schemas['ApplicationPage']>('/applications', {
      query: { status: ['moderation'], page_size: 1 },
    })

    render(<Navigation />, { wrapper: AppProviders })

    const kycLink = await screen.findByRole('link', { name: /Проверка компаний/ })
    expect(await within(kycLink).findByText(String(kyc.total))).toBeTruthy()
    const moderationLink = screen.getByRole('link', { name: /Модерация заявок/ })
    expect(await within(moderationLink).findByText(String(moderation.total))).toBeTruthy()
  })

  it('у заёмщика меню не запрашивает очереди админа', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    loginAs(BORROWER)
    render(<Navigation />, { wrapper: AppProviders })

    await screen.findByRole('link', { name: 'Мои заявки' })
    const paths = fetchSpy.mock.calls.map(([input]) => new URL(String(input), 'http://localhost').pathname)
    fetchSpy.mockRestore()
    expect(paths).not.toContain('/api/v1/admin/organizations')
    expect(paths).not.toContain('/api/v1/applications')
  })
})

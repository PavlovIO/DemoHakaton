import { beforeAll, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { Toaster } from 'sonner'
import { KycQueueTab } from './KycQueue'
import { ApplicationsModerationTab } from './ApplicationsModerationTab'
import { CompanyCard } from './CompanyCard'
import { AmlFlagsTab } from './AmlFlagsTab'
import { Navigation } from '@/components/Navigation'
import { PrivateRoute } from '@/components/PrivateRoute'
import { KycStub } from '@/pages/KycStub'
import { AppProviders } from '@/test/integration/render'
import { ADMIN, BORROWER, loginAs } from '@/test/integration/actors'
import { kycSubmittedApplicant, submitApplication } from '@/test/integration/fixtures'
import { api } from '@/lib/api-client'
import { PAGE_SIZE } from '@/lib/pagination'
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

// Админка без layout: очереди и карточка компании на своих маршрутах; меню — по запросу.
function renderAdmin(path: string, { withMenu = false } = {}) {
  loginAs(ADMIN)
  return render(
    <AppProviders initialEntries={[path]}>
      {withMenu && <Navigation />}
      <Toaster />
      <Routes>
        <Route path="/admin/kyc" element={<KycQueueTab />} />
        <Route path="/admin/applications" element={<ApplicationsModerationTab />} />
        <Route path="/admin/aml" element={<AmlFlagsTab />} />
        <Route path="/admin/organizations/:organizationId" element={<CompanyCard />} />
      </Routes>
    </AppProviders>,
  )
}

describe('карточка компании', () => {
  it('из очереди проверки админ открывает карточку и видит документы, бенефициаров, сверку с реестром', async () => {
    renderAdmin('/admin/kyc')
    fireEvent.click(await screen.findByRole('link', { name: applicant.organization.name }))

    expect(await screen.findByRole('heading', { name: applicant.organization.name })).toBeTruthy()
    const charter = await screen.findByRole('link', { name: 'charter.pdf' })
    expect(screen.getByRole('link', { name: 'beneficiaries_register.pdf' })).toBeTruthy()
    expect(screen.getByText(/Петров Пётр Петрович · RU · 100/)).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Сверка с реестром' })).toBeTruthy()
    await expectDownloadable(charter)
  })

  // Файлы сида на месте, поэтому 404 отдаём на границе сети — тем же телом, что бэкенд.
  it('файл документа недоступен — админ видит ошибку, а не битую загрузку', async () => {
    const realFetch = globalThis.fetch
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation((input, init) =>
      /\/documents\/[^/]+\/file$/.test(String(input))
        ? Promise.resolve(Response.json({ error: { code: 'NOT_FOUND', message: 'document not found' } }, { status: 404 }))
        : realFetch(input, init),
    )
    try {
      renderAdmin('/admin/organizations/55555555-5555-5555-5555-555555555555')
      fireEvent.click(await screen.findByRole('link', { name: 'Charter_Gamma.pdf' }))
      expect(await screen.findByText('Запись не найдена')).toBeTruthy()
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('в строке очереди нет кнопок решения: решение — только в карточке', async () => {
    renderAdmin('/admin/kyc')
    const row = (await screen.findByRole('link', { name: applicant.organization.name })).closest('li')!
    expect(within(row).queryByRole('button')).toBeNull()
  })

  // Ответ на подтверждение держим, затем обрываем сетевой ошибкой: до бэкенда запрос
  // не доходит, компания остаётся на проверке для следующего теста.
  it('пока подтверждение в пути, «Отклонить» неактивна', async () => {
    const realFetch = globalThis.fetch
    let dropApprove = () => {}
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation((input, init) =>
      String(input).endsWith('/kyc/approve')
        ? new Promise((_, reject) => {
            dropApprove = () => reject(new TypeError('Failed to fetch'))
          })
        : realFetch(input, init),
    )
    try {
      renderAdmin(`/admin/organizations/${applicant.organization.id}`)
      fireEvent.click(await screen.findByRole('button', { name: 'Подтвердить компанию' }))
      const reject = screen.getByRole<HTMLButtonElement>('button', { name: 'Отклонить' })
      await waitFor(() => expect(reject.disabled).toBe(true))
    } finally {
      dropApprove()
      fetchSpy.mockRestore()
    }
  })

  it('подтверждение в карточке: статус «Подтверждена», компания уходит из очереди, счётчик в меню уменьшается', async () => {
    renderAdmin('/admin/kyc', { withMenu: true })
    const kycMenuLink = await screen.findByRole('link', { name: 'Проверка компаний' })
    const queueSize = () => within(kycMenuLink).queryByText(/^\d+$/)?.textContent ?? '0'
    const before = Number((await within(kycMenuLink).findByText(/^\d+$/)).textContent)

    fireEvent.click(await screen.findByRole('link', { name: applicant.organization.name }))
    fireEvent.click(await screen.findByRole('button', { name: 'Подтвердить компанию' }))

    expect(await screen.findByText('Подтверждена')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Подтвердить компанию' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Отклонить' })).toBeNull()
    await waitFor(() => expect(queueSize()).toBe(String(before - 1)))

    fireEvent.click(screen.getByRole('link', { name: /Очередь проверки/ }))
    await waitFor(() => expect(screen.queryByRole('link', { name: applicant.organization.name })).toBeNull())
  })

  it('«Отказать» неактивна без причины; после отказа статус «Отклонена»', async () => {
    const rejected = await kycSubmittedApplicant()
    renderAdmin(`/admin/organizations/${rejected.organization.id}`)
    fireEvent.click(await screen.findByRole('button', { name: 'Отклонить' }))

    const dialog = screen.getByRole('dialog')
    const submit = within(dialog).getByRole<HTMLButtonElement>('button', { name: 'Отказать' })
    const reason = within(dialog).getByLabelText(/Причина/)
    expect(submit.disabled).toBe(true)
    fireEvent.change(reason, { target: { value: 'ок' } })
    expect(submit.disabled).toBe(true)
    fireEvent.change(reason, { target: { value: 'Устав без подписи директора' } })
    expect(submit.disabled).toBe(false)

    fireEvent.click(submit)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(await screen.findByText('Отклонена')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Отклонить' })).toBeNull()
  })

  it('несуществующая компания: «Компания не найдена»', async () => {
    renderAdmin('/admin/organizations/00000000-0000-0000-0000-000000000000')
    expect(await screen.findByText('Компания не найдена.')).toBeTruthy()
  })

  it('ошибка загрузки карточки не выдаётся за «не найдена»', async () => {
    renderAdmin('/admin/organizations/not-a-uuid')
    expect(await screen.findByText('Не удалось загрузить карточку компании.')).toBeTruthy()
    expect(screen.queryByText('Компания не найдена.')).toBeNull()
  })

  it('в модерации заявок название заёмщика ведёт в его карточку', async () => {
    renderAdmin('/admin/applications')
    const links = await screen.findAllByRole('link', { name: 'АО «Вектор»' })
    expect(links[0].getAttribute('href')).toBe('/admin/organizations/66666666-6666-6666-6666-666666666666')
  })

  it('карточка, открытая из модерации заявок, ведёт назад в модерацию', async () => {
    renderAdmin('/admin/applications')
    fireEvent.click((await screen.findAllByRole('link', { name: 'АО «Вектор»' }))[0])

    const back = await screen.findByRole('link', { name: '← Модерация заявок' })
    expect(back.getAttribute('href')).toBe('/admin/applications')
  })

  // Сид: открытые AML-флаги у Gamma GmbH и Delta Trading; тестовые компании добавляют
  // свои, так что без фильтра флаги Gamma могут не попасть на первую страницу.
  it('из карточки AML-флаги открываются только по этой компании, фильтр можно сбросить', async () => {
    renderAdmin('/admin/organizations/55555555-5555-5555-5555-555555555555')
    fireEvent.click(await screen.findByRole('link', { name: /Разобрать на вкладке AML-флагов/ }))

    await screen.findByRole('link', { name: 'Gamma GmbH' })
    const companies = within(screen.getByRole('table')).getAllByRole('link').map((link) => link.textContent)
    expect(new Set(companies)).toEqual(new Set(['Gamma GmbH']))
    const filteredTotal = Number(screen.getByText(/^Всего:/).textContent!.replace(/\D/g, ''))

    fireEvent.click(screen.getByRole('link', { name: 'Все компании' }))
    await waitFor(() =>
      expect(Number(screen.getByText(/^Всего:/).textContent!.replace(/\D/g, ''))).toBeGreaterThan(filteredTotal),
    )
  })

  it('карточка, открытая по прямой ссылке, ведёт назад в очередь проверки', async () => {
    renderAdmin('/admin/organizations/66666666-6666-6666-6666-666666666666')
    const back = await screen.findByRole('link', { name: '← Очередь проверки' })
    expect(back.getAttribute('href')).toBe('/admin/kyc')
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

  // Ниже lg подпись скрыта CSS, которого в jsdom нет: проверяем, что имя ссылки —
  // ровно подпись пункта, а не видимый текст вместе с числом бейджа.
  it('имя ссылки в меню — подпись пункта, без числа бейджа', async () => {
    loginAs(ADMIN)
    render(<Navigation />, { wrapper: AppProviders })

    const moderationLink = await screen.findByRole('link', { name: /Модерация заявок/ })
    await waitFor(() => expect(within(moderationLink).getByText(/^\d+$/)).toBeTruthy())
    expect(screen.getByRole('link', { name: 'Модерация заявок' })).toBe(moderationLink)
  })

  it('на странице очереди проверки бейдж берёт total из самой очереди, без второго запроса', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    renderAdmin('/admin/kyc', { withMenu: true })

    const kycMenuLink = await screen.findByRole('link', { name: 'Проверка компаний' })
    expect(await within(kycMenuLink).findByText(/^\d+$/)).toBeTruthy()
    const queueRequests = fetchSpy.mock.calls
      .map(([input]) => new URL(String(input), 'http://localhost'))
      .filter((url) => url.pathname === '/api/v1/admin/organizations')
    fetchSpy.mockRestore()
    expect(queueRequests.map((url) => url.searchParams.get('page_size'))).toEqual([String(PAGE_SIZE)])
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

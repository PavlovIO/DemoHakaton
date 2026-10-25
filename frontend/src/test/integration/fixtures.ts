import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'
import { ADMIN, BORROWER, LENDER, loginAs } from './actors'
import { restoreCookies, snapshotCookies } from './browser-fetch'

type Application = components['schemas']['Application']
type ApplicationPage = components['schemas']['ApplicationPage']
type Offer = components['schemas']['Offer']
type Me = components['schemas']['Me']

// Подготовка через API: проверяемое поведение — в тестах хуков, не здесь.

// Новая компания-заёмщик в draft; сессия остаётся у неё, resume() возвращает к ней
// после loginAs. Сидовые не годятся: одобрение KYC необратимо. Лимит — 10 регистраций
// с IP в час, поэтому регистраций в прогоне — по одной на сценарий, не на тест.
export async function registerApplicant() {
  const inn = String(Math.floor(Math.random() * 9e9) + 1e9)
  const me = await api.post<NonNullable<Me>>('/auth/register', {
    email: `kyc-${crypto.randomUUID()}@liquidity.local`,
    password: 'Demo-Pass-2026',
    role: 'borrower',
    organization: {
      name: `ООО «Тест ${inn}»`,
      country_code: 'RU',
      registration_number: inn,
      legal_address: '125009, г. Москва, ул. Тверская, д. 1',
    },
  })
  const session = snapshotCookies()
  return { organization: me.organization!, resume: () => restoreCookies(session) }
}

// У заёмщика минимум n заявок (досоздаёт черновики): для проверки второй страницы.
// На свежем сиде их меньше страницы, дальше прогоны переиспользуют созданные.
export async function ensureBorrowerApplications(n: number) {
  loginAs(BORROWER)
  const { total } = await api.get<ApplicationPage>('/applications', { query: { page_size: 1 } })
  for (let i = total; i < n; i++) {
    await api.post<Application>('/applications', {
      amount: '50000.00',
      currency: 'EUR',
      target_country_code: 'DE',
      term_months: 3,
      transfer_method: 'sepa',
      purpose: `Черновик для пагинации ${crypto.randomUUID()}`,
    })
  }
}

export async function submitApplication() {
  loginAs(BORROWER)
  const draft = await api.post<Application>('/applications', {
    amount: '100000.00',
    currency: 'EUR',
    target_country_code: 'DE',
    term_months: 12,
    transfer_method: 'sepa',
    purpose: `Пополнение оборотных средств ${crypto.randomUUID()}`,
  })
  return api.post<Application>('/applications/{applicationId}/submit', undefined, { path: { applicationId: draft.id } })
}

const REQUIRED_DOCS = ['charter', 'registration_certificate', 'address_proof', 'beneficiaries_register'] as const

// Новая компания со всем, что нужно для отправки KYC.
export async function kycReadyApplicant() {
  const applicant = await registerApplicant()
  for (const docType of REQUIRED_DOCS) {
    const form = new FormData()
    form.append('doc_type', docType)
    form.append('file', new File(['%PDF-1.4\n'], `${docType}.pdf`, { type: 'application/pdf' }))
    await api.post('/organizations/me/documents', form)
  }
  await api.patch('/organizations/me', {
    beneficiaries: [{ full_name: 'Петров Пётр Петрович', citizenship_country_code: 'RU', ownership_percent: '100' }],
  })
  return applicant
}

// Компания в очереди верификации; сессия остаётся у неё.
export async function kycSubmittedApplicant() {
  const applicant = await kycReadyApplicant()
  await api.post('/organizations/me/kyc/submit')
  return applicant
}

export async function publishApplication() {
  const app = await submitApplication()
  loginAs(ADMIN)
  return api.post<Application>('/admin/applications/{applicationId}/approve', undefined, { path: { applicationId: app.id } })
}

// Опубликованная заявка с оффером кредитора; сессия остаётся у кредитора.
export async function sendOffer() {
  const app = await publishApplication()
  loginAs(LENDER)
  const offer = await api.post<Offer>(
    '/applications/{applicationId}/offers',
    { amount: '90000.00', interest_rate_annual: '9.50', fee_percent: '1.00', repayment_schedule: 'bullet' },
    { path: { applicationId: app.id } },
  )
  return { app, offer }
}

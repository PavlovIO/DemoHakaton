import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'
import { ADMIN, BORROWER, LENDER, loginAs } from './actors'

type Application = components['schemas']['Application']
type Offer = components['schemas']['Offer']

// Подготовка через API: проверяемое поведение — в тестах хуков, не здесь.

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

import { describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import {
  useApproveApplication,
  useCreateApplication,
  useMarketplace,
  useModerationQueue,
  useMyApplications,
  useRejectApplication,
  useSubmitApplication,
} from './useApplications'
import { AppProviders } from '@/test/integration/render'
import { ADMIN, BORROWER, LENDER, loginAs } from '@/test/integration/actors'
import { ensureBorrowerApplications, publishApplication, sendOffer, submitApplication } from '@/test/integration/fixtures'
import { api } from '@/lib/api-client'
import { PAGE_SIZE } from '@/lib/pagination'

describe('заявки против живого бэкенда', () => {
  it('заёмщик сохраняет черновик и подаёт его — заявка в его списке на модерации', async () => {
    loginAs(BORROWER)
    const purpose = `Закупка оборудования ${crypto.randomUUID()}`

    const { result } = renderHook(
      () => ({ create: useCreateApplication(), submit: useSubmitApplication(), list: useMyApplications() }),
      { wrapper: AppProviders },
    )
    const draft = await act(() =>
      result.current.create.mutateAsync({
        amount: '250000.00',
        currency: 'EUR',
        target_country_code: 'DE',
        term_months: 6,
        transfer_method: 'swift',
        purpose,
      }),
    )
    // Черновик виден сразу: если подача сорвётся, его можно подать повторно из списка.
    await waitFor(() =>
      expect(result.current.list.data?.items).toContainEqual(
        expect.objectContaining({ id: draft.id, purpose, status: 'draft' }),
      ),
    )

    await act(() => result.current.submit.mutateAsync(draft.id))

    await waitFor(() =>
      expect(result.current.list.data?.items).toContainEqual(
        expect.objectContaining({ id: draft.id, status: 'moderation', amount: '250000.00' }),
      ),
    )
  })

  it('admin публикует заявку из очереди — кредитор видит её на витрине', async () => {
    const app = await submitApplication()

    loginAs(ADMIN)
    const admin = renderHook(
      () => ({ queue: useModerationQueue(), approve: useApproveApplication() }),
      { wrapper: AppProviders },
    )
    await waitFor(() =>
      expect(admin.result.current.queue.data?.items.map((a) => a.id)).toContain(app.id),
    )
    await act(() => admin.result.current.approve.mutateAsync(app.id))
    admin.unmount()

    loginAs(LENDER)
    const lender = renderHook(() => useMarketplace({ currency: 'EUR' }), { wrapper: AppProviders })
    await waitFor(() =>
      expect(lender.result.current.data?.items).toContainEqual(
        expect.objectContaining({ id: app.id, status: 'published' }),
      ),
    )
  })
})

describe('витрина кредитора', () => {
  it('согласованная заявка уходит с витрины, хотя по ней есть мой оффер', async () => {
    const { app, offer } = await sendOffer()
    loginAs(BORROWER)
    await api.post('/offers/{offerId}/accept', undefined, { path: { offerId: offer.id } })

    loginAs(LENDER)
    const { result } = renderHook(() => useMarketplace({ currency: 'EUR' }), { wrapper: AppProviders })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data!.items.map((a) => a.id)).not.toContain(app.id)
    expect(result.current.data!.items.every((a) => a.status === 'published')).toBe(true)
  })

  it('«сумма от» в валюте отсекает заявки меньше порога', async () => {
    const app = await publishApplication() // 100 000,00 EUR

    loginAs(LENDER)
    const above = renderHook(() => useMarketplace({ currency: 'EUR', amount_min: '100000.01' }), { wrapper: AppProviders })
    const atLeast = renderHook(() => useMarketplace({ currency: 'EUR', amount_min: '100000.00' }), { wrapper: AppProviders })
    await waitFor(() => {
      expect(above.result.current.isSuccess).toBe(true)
      expect(atLeast.result.current.data?.items.map((a) => a.id)).toContain(app.id)
    })
    expect(above.result.current.data!.items.map((a) => a.id)).not.toContain(app.id)
  })
})

describe('модерация: отказ', () => {
  it('admin возвращает заявку с причиной — заёмщик видит отказ и причину', async () => {
    const app = await submitApplication()

    loginAs(ADMIN)
    const admin = renderHook(() => useRejectApplication(), { wrapper: AppProviders })
    await act(() => admin.result.current.mutateAsync({ id: app.id, reason: 'Уточните назначение платежа' }))
    admin.unmount()

    loginAs(BORROWER)
    const borrower = renderHook(() => useMyApplications(), { wrapper: AppProviders })
    await waitFor(() =>
      expect(borrower.result.current.data?.items).toContainEqual(
        expect.objectContaining({
          id: app.id,
          status: 'rejected',
          moderation_comment: 'Уточните назначение платежа',
        }),
      ),
    )
  })
})

describe('пагинация', () => {
  it('«показать ещё» догружает следующую страницу без повторов', async () => {
    await ensureBorrowerApplications(PAGE_SIZE + 1)

    const { result } = renderHook(() => useMyApplications(), { wrapper: AppProviders })
    await waitFor(() => expect(result.current.data?.items).toHaveLength(PAGE_SIZE))
    expect(result.current.data!.total).toBeGreaterThan(PAGE_SIZE)
    expect(result.current.hasNextPage).toBe(true)
    const firstPage = result.current.data!.items.map((a) => a.id)

    await act(() => result.current.fetchNextPage())

    await waitFor(() => expect(result.current.data!.items.length).toBeGreaterThan(PAGE_SIZE))
    const ids = result.current.data!.items.map((a) => a.id)
    expect(ids.slice(0, PAGE_SIZE)).toEqual(firstPage)
    expect(new Set(ids).size).toBe(ids.length)
  })
})

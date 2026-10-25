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
import { submitApplication } from '@/test/integration/fixtures'

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

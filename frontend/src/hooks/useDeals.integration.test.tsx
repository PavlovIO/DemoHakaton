import { describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useDeals } from './useDeals'
import { useAcceptOffer } from './useOffers'
import { useMyApplications } from './useApplications'
import { AppProviders } from '@/test/integration/render'
import { BORROWER, LENDER, loginAs } from '@/test/integration/actors'
import { sendOffer } from '@/test/integration/fixtures'

describe('сделки против живого бэкенда', () => {
  it('заёмщик принимает оффер — заявка согласована, сделка видна обеим сторонам', async () => {
    const { app, offer } = await sendOffer()

    loginAs(BORROWER)
    const borrower = renderHook(
      () => ({ accept: useAcceptOffer(), deals: useDeals(), apps: useMyApplications() }),
      { wrapper: AppProviders },
    )
    const deal = await act(() => borrower.result.current.accept.mutateAsync(offer.id))

    await waitFor(() => {
      expect(borrower.result.current.deals.data?.items).toContainEqual(
        expect.objectContaining({ id: deal.id, offer_id: offer.id, amount: '90000.00' }),
      )
      expect(borrower.result.current.apps.data?.items).toContainEqual(
        expect.objectContaining({ id: app.id, status: 'agreed' }),
      )
    })
    borrower.unmount()

    loginAs(LENDER)
    const lender = renderHook(() => useDeals(), { wrapper: AppProviders })
    await waitFor(() =>
      expect(lender.result.current.data?.items.map((d) => d.id)).toContain(deal.id),
    )
  })
})

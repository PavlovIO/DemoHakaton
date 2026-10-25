import { describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useApplicationOffers, useCreateOffer, useMyOffers, useWithdrawOffer } from './useOffers'
import { AppProviders } from '@/test/integration/render'
import { BORROWER, LENDER, loginAs } from '@/test/integration/actors'
import { publishApplication, sendOffer } from '@/test/integration/fixtures'

describe('офферы против живого бэкенда', () => {
  it('кредитор отправляет оффер — он у кредитора в «Моих предложениях» и у заёмщика в заявке', async () => {
    const app = await publishApplication()

    loginAs(LENDER)
    const lender = renderHook(
      () => ({ create: useCreateOffer(), mine: useMyOffers() }),
      { wrapper: AppProviders },
    )
    const offer = await act(() =>
      lender.result.current.create.mutateAsync({
        applicationId: app.id,
        amount: '80000.00',
        interest_rate_annual: '8.00',
        fee_percent: '1.00',
        repayment_schedule: 'annuity',
      }),
    )
    await waitFor(() =>
      expect(lender.result.current.mine.data?.items).toContainEqual(
        expect.objectContaining({ id: offer.id, status: 'sent', application_number: app.number }),
      ),
    )
    lender.unmount()

    loginAs(BORROWER)
    const borrower = renderHook(() => useApplicationOffers(app.id), { wrapper: AppProviders })
    await waitFor(() =>
      expect(borrower.result.current.data).toEqual([
        expect.objectContaining({ id: offer.id, amount: '80000.00', interest_rate_annual: '8.0000' }),
      ]),
    )
  })

  it('кредитор отзывает оффер — статус «отозван»', async () => {
    const { offer } = await sendOffer()

    const { result } = renderHook(
      () => ({ withdraw: useWithdrawOffer(), mine: useMyOffers() }),
      { wrapper: AppProviders },
    )
    await act(() => result.current.withdraw.mutateAsync(offer.id))

    await waitFor(() =>
      expect(result.current.mine.data?.items).toContainEqual(
        expect.objectContaining({ id: offer.id, status: 'withdrawn' }),
      ),
    )
  })
})

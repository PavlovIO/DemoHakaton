import { describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useMessages, useSendMessage } from './useMessages'
import { AppProviders } from '@/test/integration/render'
import { BORROWER, loginAs } from '@/test/integration/actors'
import { sendOffer } from '@/test/integration/fixtures'

describe('чат по офферу против живого бэкенда', () => {
  it('кредитор пишет — заёмщик видит сообщение собеседника', async () => {
    const { offer } = await sendOffer()

    const lender = renderHook(
      () => ({ send: useSendMessage(offer.id), messages: useMessages(offer.id) }),
      { wrapper: AppProviders },
    )
    await act(() => lender.result.current.send.mutateAsync('Готовы обсудить график?'))
    await waitFor(() =>
      expect(lender.result.current.messages.data).toEqual([
        expect.objectContaining({ text: 'Готовы обсудить график?', is_mine: true, author_role: 'lender' }),
      ]),
    )
    lender.unmount()

    loginAs(BORROWER)
    const borrower = renderHook(() => useMessages(offer.id), { wrapper: AppProviders })
    await waitFor(() =>
      expect(borrower.result.current.data).toEqual([
        expect.objectContaining({ text: 'Готовы обсудить график?', is_mine: false }),
      ]),
    )
  })
})

import { describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useMessages, useSendMessage, type Message } from './useMessages'
import { AppProviders } from '@/test/integration/render'
import { BORROWER, LENDER, loginAs } from '@/test/integration/actors'
import { sendOffer } from '@/test/integration/fixtures'
import { api } from '@/lib/api-client'

describe('чат по офферу против живого бэкенда', () => {
  it('кредитор пишет — заёмщик видит сообщение собеседника', async () => {
    const { offer } = await sendOffer()

    const lender = renderHook(
      () => ({ send: useSendMessage(offer.id), messages: useMessages(offer.id) }),
      { wrapper: AppProviders },
    )
    // Список загружен до мутации, как в UI: иначе invalidateQueries присоединится к первому
    // GET, ушедшему до мутации, и список останется устаревшим.
    await waitFor(() => expect(lender.result.current.messages.isSuccess).toBe(true))
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

  it('опрос дозапрашивает только новые сообщения после последнего в кэше', async () => {
    const { offer } = await sendOffer()
    const first = await api.post<Message>('/offers/{offerId}/messages', { text: 'Первое' }, { path: { offerId: offer.id } })

    loginAs(BORROWER)
    const borrower = renderHook(() => useMessages(offer.id), { wrapper: AppProviders })
    await waitFor(() => expect(borrower.result.current.data).toHaveLength(1))

    loginAs(LENDER)
    await api.post('/offers/{offerId}/messages', { text: 'Второе' }, { path: { offerId: offer.id } })
    loginAs(BORROWER)

    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    try {
      await act(() => borrower.result.current.refetch())
      const polled = fetchSpy.mock.calls.map(([input]) => new URL(String(input), 'http://localhost'))
      expect(polled).toEqual([expect.objectContaining({ pathname: `/api/v1/offers/${offer.id}/messages` })])
      expect(polled[0].searchParams.get('after_id')).toBe(String(first.id))
    } finally {
      fetchSpy.mockRestore()
    }
    await waitFor(() => expect(borrower.result.current.data?.map((m) => m.text)).toEqual(['Первое', 'Второе']))
  })
})

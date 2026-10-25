import { describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useMarkAllNotificationsRead, useNotifications } from './useNotifications'
import { AppProviders } from '@/test/integration/render'
import { BORROWER, loginAs } from '@/test/integration/actors'
import { sendOffer } from '@/test/integration/fixtures'

describe('уведомления против живого бэкенда', () => {
  it('заёмщик получает уведомление о новом оффере и отмечает все прочитанными', async () => {
    const { offer } = await sendOffer()

    loginAs(BORROWER)
    const { result } = renderHook(
      () => ({ list: useNotifications(), readAll: useMarkAllNotificationsRead() }),
      { wrapper: AppProviders },
    )
    await waitFor(() =>
      expect(result.current.list.data?.items).toContainEqual(
        expect.objectContaining({ entity_id: offer.id, is_read: false }),
      ),
    )

    await act(() => result.current.readAll.mutateAsync())

    await waitFor(() => expect(result.current.list.data?.unread_total).toBe(0))
  })
})

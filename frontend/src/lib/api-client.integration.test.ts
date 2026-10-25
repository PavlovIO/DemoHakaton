import { describe, expect, it } from 'vitest'
import { api, ApiError } from './api-client'
import type { components } from './api-schema'

type Me = components['schemas']['Me']

describe('api-client против живого бэкенда', () => {
  it('неверный пароль — ApiError с кодом и сообщением бэкенда', async () => {
    const err = await api
      .post('/auth/login', { email: 'nobody@liquidity.local', password: 'Wrong-Pass-2026' })
      .catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({
      status: 401,
      code: 'UNAUTHORIZED',
      message: 'Неверный email или пароль',
    })
  })

  it('нарушение контракта — VALIDATION_ERROR с ошибками по полям', async () => {
    const err = await api
      .post('/auth/register', {
        email: 'short-pass@liquidity.local',
        password: 'short',
        role: 'borrower',
        organization: { name: 'ООО «Тест»', country_code: 'RU', registration_number: '7700000009' },
      })
      .catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({
      status: 400,
      code: 'VALIDATION_ERROR',
      details: {
        password: ['недопустимая длина'],
        'organization.legal_address': ['обязательное поле'],
      },
    })
  })

  it('сессия: вход → /auth/me → выход с CSRF-токеном → сессии нет', async () => {
    await api.post('/auth/login', { email: 'beta@liquidity.local', password: 'Demo-Pass-2026' })

    const me = await api.get<Me>('/auth/me')
    expect(me.user.email).toBe('beta@liquidity.local')
    expect(me.user.role).toBe('lender')

    // Без X-CSRF-Token бэкенд ответил бы 403 CSRF_INVALID.
    await expect(api.post('/auth/logout')).resolves.toBeUndefined()

    await expect(api.get('/auth/me')).rejects.toMatchObject({ status: 401, code: 'UNAUTHORIZED' })
  })
})

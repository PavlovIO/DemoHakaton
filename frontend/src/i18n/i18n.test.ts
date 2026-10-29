// @vitest-environment jsdom
import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { components } from '@/lib/api-schema'
import { kycStatusLabel, roleLabel } from '@/lib/labels'
import { en } from './en'
import { currentLanguage, currentLocale, setLanguage, useRoleLanguage } from './index'
import { ru } from './ru'

type Role = components['schemas']['Role']

// Пути ко всем листьям словаря: 'kyc.documents.type.other' → значение.
function leaves(dict: object, prefix = ''): Map<string, unknown> {
  const out = new Map<string, unknown>()
  for (const [key, value] of Object.entries(dict)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (value && typeof value === 'object') {
      for (const [p, v] of leaves(value, path)) out.set(p, v)
    } else {
      out.set(path, value)
    }
  }
  return out
}

describe('словари i18n', () => {
  it('en повторяет все ключи ru, лишних ключей нет', () => {
    expect([...leaves(en).keys()].sort()).toEqual([...leaves(ru).keys()].sort())
  })

  it('все переводы — непустые строки', () => {
    for (const dict of [ru, en]) {
      for (const [path, value] of leaves(dict)) {
        expect(typeof value === 'string' && value.trim() !== '', path).toBe(true)
      }
    }
  })
})

describe('переключение языка', () => {
  afterEach(() => setLanguage('ru'))

  it('по умолчанию русский', () => {
    expect(currentLanguage()).toBe('ru')
    expect(kycStatusLabel('verified')).toBe('Подтверждена')
  })

  it('setLanguage("en") переводит подписи и запоминает выбор', () => {
    setLanguage('en')

    expect(kycStatusLabel('verified')).toBe('Verified')
    expect(roleLabel('lender')).toBe('Lender')
    expect(currentLocale()).toBe('en-GB')
    expect(localStorage.getItem('lp.language')).toBe('en')
    expect(document.documentElement.lang).toBe('en')
  })

  it('setLanguage("ru") возвращает русский', () => {
    setLanguage('en')
    setLanguage('ru')

    expect(roleLabel('lender')).toBe('Кредитор')
    expect(localStorage.getItem('lp.language')).toBe('ru')
    expect(document.documentElement.lang).toBe('ru')
  })
})

describe('useRoleLanguage', () => {
  afterEach(() => setLanguage('ru'))

  it('администратору — русский, выбор пользователя сохраняется и возвращается', () => {
    setLanguage('en')
    const { rerender } = renderHook(({ role }) => useRoleLanguage(role), {
      initialProps: { role: 'admin' as Role | undefined },
    })

    expect(currentLanguage()).toBe('ru')
    expect(localStorage.getItem('lp.language')).toBe('en')

    rerender({ role: undefined })
    expect(currentLanguage()).toBe('en')
  })

  it('заёмщику — русский, выбор в браузере не затирается', () => {
    setLanguage('en')
    renderHook(() => useRoleLanguage('borrower'))

    expect(currentLanguage()).toBe('ru')
    expect(kycStatusLabel('verified')).toBe('Подтверждена')
    expect(localStorage.getItem('lp.language')).toBe('en')
  })

  it('кредитору — выбранный язык', () => {
    setLanguage('en')
    renderHook(() => useRoleLanguage('lender'))

    expect(currentLanguage()).toBe('en')
    expect(kycStatusLabel('verified')).toBe('Verified')
  })
})

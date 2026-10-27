import { describe, expect, it } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useCountries, useCurrencies, useTransferMethods, pickDefaultOption } from './useDictionaries'
import { AppProviders } from '@/test/integration/render'

describe('справочники против живого бэкенда', () => {
  it('список стран шире прежнего хардкода и содержит DE с флагом FATF', async () => {
    const { result } = renderHook(() => useCountries(), { wrapper: AppProviders })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const countries = result.current.data!
    expect(countries.length).toBeGreaterThan(3)
    const de = countries.find((c) => c.code === 'DE')
    expect(de).toMatchObject({ code: 'DE', name_ru: expect.any(String) })
    expect(typeof de!.fatf_high_risk).toBe('boolean')
  })

  it('валюты покрывают коды, используемые в заявках (EUR/USD/RUB)', async () => {
    const { result } = renderHook(() => useCurrencies(), { wrapper: AppProviders })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const codes = result.current.data!.map((c) => c.code)
    expect(codes).toEqual(expect.arrayContaining(['EUR', 'USD', 'RUB']))
  })

  it('способы перевода содержат enum контракта swift/sepa', async () => {
    const { result } = renderHook(() => useTransferMethods(), { wrapper: AppProviders })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const codes = result.current.data!.map((m) => m.code)
    expect(codes).toEqual(expect.arrayContaining(['swift', 'sepa']))
  })
})

describe('pickDefaultOption', () => {
  const options = [{ code: 'EUR' }, { code: 'USD' }]

  it('возвращает предпочтённый код, если он есть в справочнике', () => {
    expect(pickDefaultOption(options, 'USD')).toEqual({ code: 'USD' })
  })

  it('берёт первую опцию, если предпочтённого кода в справочнике нет', () => {
    expect(pickDefaultOption(options, 'RUB')).toEqual({ code: 'EUR' })
  })

  it('возвращает undefined на пустом справочнике — форма тогда покажет fallback', () => {
    expect(pickDefaultOption([], 'EUR')).toBeUndefined()
  })
})
import { useQuery, type UseQueryResult } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { operations } from '@/lib/api-schema'

export type Country = operations['listCountries']['responses'][200]['content']['application/json'][number]
export type Currency = operations['listCurrencies']['responses'][200]['content']['application/json'][number]
export type TransferMethodItem = operations['listTransferMethods']['responses'][200]['content']['application/json'][number]

export function pickDefaultOption<T extends { code: string }>(
  options: T[],
  preferredCode: string,
): T | undefined {
  return options.find((o) => o.code === preferredCode) ?? options[0]
}

const DICTIONARIES_STALE_TIME_MS = 24 * 60 * 60 * 1000

export function useCountries(): UseQueryResult<Country[]> {
  return useQuery({
    queryKey: ['dictionaries', 'countries'],
    queryFn: ({ signal }) => api.get<Country[]>('/dictionaries/countries', { signal }),
    staleTime: DICTIONARIES_STALE_TIME_MS,
  })
}

export function useCurrencies(): UseQueryResult<Currency[]> {
  return useQuery({
    queryKey: ['dictionaries', 'currencies'],
    queryFn: ({ signal }) => api.get<Currency[]>('/dictionaries/currencies', { signal }),
    staleTime: DICTIONARIES_STALE_TIME_MS,
  })
}

export function useTransferMethods(): UseQueryResult<TransferMethodItem[]> {
  return useQuery({
    queryKey: ['dictionaries', 'transfer-methods'],
    queryFn: ({ signal }) => api.get<TransferMethodItem[]>('/dictionaries/transfer-methods', { signal }),
    staleTime: DICTIONARIES_STALE_TIME_MS,
  })
}
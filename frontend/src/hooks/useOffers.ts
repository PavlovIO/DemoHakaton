import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'
import { PAGE_SIZE, pagedList } from '@/lib/pagination'
import { APPLICATIONS_KEY } from './useApplications'
import { DEALS_KEY, type Deal } from './useDeals'

export type Offer = components['schemas']['Offer']
export type OfferCreate = components['schemas']['OfferCreate']
type OfferPage = components['schemas']['OfferPage']

export const OFFERS_KEY = ['offers'] as const

// Кредитору бэкенд отдаёт офферы его организации.
export function useMyOffers() {
  return useInfiniteQuery({
    queryKey: [...OFFERS_KEY, 'mine'],
    queryFn: ({ pageParam }) => api.get<OfferPage>('/offers', { query: { page: pageParam, page_size: PAGE_SIZE } }),
    ...pagedList<Offer>(),
  })
}

export function useApplicationOffers(applicationId: string | undefined) {
  return useQuery({
    queryKey: [...OFFERS_KEY, 'application', applicationId],
    queryFn: () =>
      api.get<Offer[]>('/applications/{applicationId}/offers', { path: { applicationId } }),
    enabled: !!applicationId,
  })
}

// Принятие фиксирует сделку; остальные офферы по заявке бэкенд отклоняет сам.
export function useAcceptOffer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (offerId: string) =>
      api.post<Deal>('/offers/{offerId}/accept', undefined, { path: { offerId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: OFFERS_KEY })
      queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY })
      queryClient.invalidateQueries({ queryKey: DEALS_KEY })
    },
  })
}

export function useWithdrawOffer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (offerId: string) =>
      api.post<Offer>('/offers/{offerId}/withdraw', undefined, { path: { offerId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: OFFERS_KEY })
      queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY })
    },
  })
}

export function useCreateOffer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ applicationId, ...payload }: OfferCreate & { applicationId: string }) =>
      api.post<Offer>('/applications/{applicationId}/offers', payload, { path: { applicationId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: OFFERS_KEY })
      // У заявки меняется my_offer_id — витрина показывает «Оффер отправлен».
      queryClient.invalidateQueries({ queryKey: APPLICATIONS_KEY })
    },
  })
}

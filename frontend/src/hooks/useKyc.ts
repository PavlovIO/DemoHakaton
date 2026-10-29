import { type QueryClient, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api-client'
import type { components } from '@/lib/api-schema'
import { PAGE_SIZE, pagedList } from '@/lib/pagination'
import { ME_KEY } from './meKey'

export type KycDocument = components['schemas']['KycDocument']
export type KycDocumentType = components['schemas']['KycDocumentType']
export type Beneficiary = components['schemas']['Beneficiary']
type Organization = components['schemas']['Organization']
type OrganizationPage = components['schemas']['OrganizationPage']
export type OrganizationDetails = components['schemas']['OrganizationDetails']

const DOCUMENTS_KEY = ['kyc-documents'] as const
const ORGANIZATION_KEY = ['my-organization'] as const
const KYC_QUEUE_KEY = ['kyc-queue'] as const
const COMPANY_CARD_KEY = ['company-card'] as const
export { KYC_QUEUE_KEY, COMPANY_CARD_KEY }

export function useMyDocuments() {
  return useQuery({
    queryKey: DOCUMENTS_KEY,
    queryFn: () => api.get<KycDocument[]>('/organizations/me/documents'),
  })
}

// Файл документа отдаётся и владельцу, и администратору. Адрес — для href ссылки
// (открыть в новой вкладке), скачивание по клику — через downloadDocumentFile.
export function documentFileUrl(id: string) {
  return api.url('/documents/{documentId}/file', { path: { documentId: id } })
}

export function downloadDocumentFile(id: string) {
  return api.blob('/documents/{documentId}/file', { path: { documentId: id } })
}

export function useUploadDocument() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ docType, file }: { docType: KycDocumentType; file: File }) => {
      // doc_type раньше file: сервер проверяет тип документа до приёма файла.
      const form = new FormData()
      form.append('doc_type', docType)
      form.append('file', file)
      return api.post<KycDocument>('/organizations/me/documents', form)
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: DOCUMENTS_KEY }),
  })
}

export function useDeleteDocument() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api.delete<void>('/organizations/me/documents/{documentId}', { path: { documentId: id } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: DOCUMENTS_KEY }),
  })
}

// Полный профиль: в /auth/me организация без бенефициаров.
export function useMyOrganization() {
  return useQuery({
    queryKey: ORGANIZATION_KEY,
    queryFn: () => api.get<Organization>('/organizations/me'),
  })
}

// Список заменяется целиком: PATCH с beneficiaries перезаписывает всех.
export function useUpdateBeneficiaries() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (beneficiaries: Beneficiary[]) =>
      api.patch<Organization>('/organizations/me', { beneficiaries }),
    onSuccess: () => invalidateOrganization(queryClient),
  })
}

function invalidateOrganization(queryClient: QueryClient) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEY }),
    queryClient.invalidateQueries({ queryKey: ME_KEY }),
  ])
}

// draft|rejected → under_review; статус в /auth/me управляет доступом к платформе.
export function useSubmitKyc() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<Organization>('/organizations/me/kyc/submit'),
    onSuccess: () => invalidateOrganization(queryClient),
  })
}

// Компании, ждущие решения администратора; счётчик в меню считает тем же фильтром.
const KYC_QUEUE_FILTER = { kyc_status: ['under_review'] }

export function useKycQueue() {
  return useInfiniteQuery({
    queryKey: KYC_QUEUE_KEY,
    queryFn: ({ pageParam }) =>
      api.get<OrganizationPage>('/admin/organizations', {
        query: { ...KYC_QUEUE_FILTER, page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<OrganizationPage>(),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

// Размер очереди: total считает бэкенд, строки не нужны. Ключ под KYC_QUEUE_KEY —
// решения по компаниям сбрасывают и его.
export function useKycQueueCount() {
  return useQuery({
    queryKey: [...KYC_QUEUE_KEY, 'count'],
    queryFn: () =>
      api.get<OrganizationPage>('/admin/organizations', { query: { ...KYC_QUEUE_FILTER, page_size: 1 } }),
    select: (page) => page.total,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
}

// Карточка компании: всё, что администратор смотрит перед решением по проверке.
export function useCompanyCard(organizationId: string) {
  return useQuery({
    queryKey: [...COMPANY_CARD_KEY, organizationId],
    queryFn: () =>
      api.get<OrganizationDetails>('/admin/organizations/{organizationId}', { path: { organizationId } }),
  })
}

// Решение меняет и очередь (компания уходит из неё), и карточку (статус, кнопки).
function invalidateKycDecision(queryClient: QueryClient) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: KYC_QUEUE_KEY }),
    queryClient.invalidateQueries({ queryKey: COMPANY_CARD_KEY }),
  ])
}

export function useApproveKyc() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (organizationId: string) =>
      api.post<Organization>('/admin/organizations/{organizationId}/kyc/approve', undefined, {
        path: { organizationId },
      }),
    onSuccess: () => invalidateKycDecision(queryClient),
  })
}

export function useRejectKyc() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      api.post<Organization>(
        '/admin/organizations/{organizationId}/kyc/reject',
        { reason },
        { path: { organizationId: id } },
      ),
    onSuccess: () => invalidateKycDecision(queryClient),
  })
}

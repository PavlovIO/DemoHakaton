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

const DOCUMENTS_KEY = ['kyc-documents'] as const
const ORGANIZATION_KEY = ['my-organization'] as const
const KYC_QUEUE_KEY = ['kyc-queue'] as const

export function useMyDocuments() {
  return useQuery({
    queryKey: DOCUMENTS_KEY,
    queryFn: () => api.get<KycDocument[]>('/organizations/me/documents'),
  })
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

export function useKycQueue() {
  return useInfiniteQuery({
    queryKey: KYC_QUEUE_KEY,
    queryFn: ({ pageParam }) =>
      api.get<OrganizationPage>('/admin/organizations', {
        query: { kyc_status: ['under_review'], page: pageParam, page_size: PAGE_SIZE },
      }),
    ...pagedList<Organization>(),
  })
}

export function useApproveKyc() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (organizationId: string) =>
      api.post<Organization>('/admin/organizations/{organizationId}/kyc/approve', undefined, {
        path: { organizationId },
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KYC_QUEUE_KEY }),
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
    onSuccess: () => queryClient.invalidateQueries({ queryKey: KYC_QUEUE_KEY }),
  })
}

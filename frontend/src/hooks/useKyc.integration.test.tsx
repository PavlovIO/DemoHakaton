import { beforeAll, describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import {
  useDeleteDocument,
  useApproveKyc,
  useKycQueue,
  useMyDocuments,
  useMyOrganization,
  useRejectKyc,
  useSubmitKyc,
  useUpdateBeneficiaries,
  useUploadDocument,
} from './useKyc'
import { useAuth } from './useAuth'
import { AppProviders } from '@/test/integration/render'
import { ADMIN, loginAs } from '@/test/integration/actors'
import { kycReadyApplicant, kycSubmittedApplicant, registerApplicant } from '@/test/integration/fixtures'

// Бэкенд определяет тип по содержимому, а не по расширению.
const pdf = (name: string) => new File(['%PDF-1.4\n%тест\n'], name, { type: 'application/pdf' })

describe('подготовка к KYC против живого бэкенда', () => {
  // Одна компания в draft на все тесты блока: лимит регистраций — 10 в час.
  let applicant: Awaited<ReturnType<typeof registerApplicant>>
  beforeAll(async () => {
    applicant = await registerApplicant()
  })

  it('заявитель загружает устав — документ в его списке', async () => {
    applicant.resume()

    const { result } = renderHook(
      () => ({ upload: useUploadDocument(), list: useMyDocuments() }),
      { wrapper: AppProviders },
    )
    await act(() => result.current.upload.mutateAsync({ docType: 'charter', file: pdf('устав.pdf') }))

    await waitFor(() =>
      expect(result.current.list.data).toContainEqual(
        expect.objectContaining({ doc_type: 'charter', file_name: 'устав.pdf', mime_type: 'application/pdf' }),
      ),
    )
  })

  it('заявитель удаляет ошибочно загруженный документ — его больше нет в списке', async () => {
    applicant.resume()

    const { result } = renderHook(
      () => ({ upload: useUploadDocument(), remove: useDeleteDocument(), list: useMyDocuments() }),
      { wrapper: AppProviders },
    )
    const wrong = await act(() =>
      result.current.upload.mutateAsync({ docType: 'address_proof', file: pdf('не-тот-файл.pdf') }),
    )
    await waitFor(() => expect(result.current.list.data?.map((d) => d.id)).toContain(wrong.id))

    await act(() => result.current.remove.mutateAsync(wrong.id))

    await waitFor(() => expect(result.current.list.data?.map((d) => d.id)).not.toContain(wrong.id))
  })

  it('заявитель указывает бенефициара — он в профиле компании', async () => {
    applicant.resume()

    const { result } = renderHook(
      () => ({ save: useUpdateBeneficiaries(), profile: useMyOrganization() }),
      { wrapper: AppProviders },
    )
    await waitFor(() => expect(result.current.profile.isSuccess).toBe(true))

    await act(() =>
      result.current.save.mutateAsync([
        { full_name: 'Иванов Иван Иванович', citizenship_country_code: 'RU', ownership_percent: '75' },
      ]),
    )

    await waitFor(() =>
      expect(result.current.profile.data?.beneficiaries).toEqual([
        { full_name: 'Иванов Иван Иванович', citizenship_country_code: 'RU', ownership_percent: '75.0000' },
      ]),
    )
  })
})

describe('верификация компании против живого бэкенда', () => {
  it('заявитель отправляет KYC, admin одобряет из очереди — компания верифицирована', async () => {
    const applicant = await kycReadyApplicant()

    const { result, unmount } = renderHook(() => ({ submit: useSubmitKyc(), auth: useAuth() }), {
      wrapper: AppProviders,
    })
    await waitFor(() => expect(result.current.auth.organization?.kyc_status).toBe('draft'))
    await act(() => result.current.submit.mutateAsync())
    await waitFor(() => expect(result.current.auth.organization?.kyc_status).toBe('under_review'))
    unmount()

    loginAs(ADMIN)
    const admin = renderHook(() => ({ queue: useKycQueue(), approve: useApproveKyc() }), { wrapper: AppProviders })
    await waitFor(() =>
      expect(admin.result.current.queue.data?.items).toContainEqual(
        expect.objectContaining({ id: applicant.organization.id, kyc_status: 'under_review' }),
      ),
    )
    await act(() => admin.result.current.approve.mutateAsync(applicant.organization.id))
    // Одобренная компания уходит из очереди.
    await waitFor(() =>
      expect(admin.result.current.queue.data?.items.map((o) => o.id)).not.toContain(applicant.organization.id),
    )
    admin.unmount()

    applicant.resume()
    const after = renderHook(() => useAuth(), { wrapper: AppProviders })
    await waitFor(() => expect(after.result.current.isVerified).toBe(true))
  })

  it('admin отклоняет KYC с причиной — заявитель видит отказ и причину', async () => {
    const applicant = await kycSubmittedApplicant()

    loginAs(ADMIN)
    const admin = renderHook(() => useRejectKyc(), { wrapper: AppProviders })
    await act(() =>
      admin.result.current.mutateAsync({ id: applicant.organization.id, reason: 'Устав не заверен нотариально' }),
    )
    admin.unmount()

    applicant.resume()
    const after = renderHook(() => useAuth(), { wrapper: AppProviders })
    await waitFor(() =>
      expect(after.result.current.organization).toMatchObject({
        kyc_status: 'rejected',
        kyc_reject_reason: 'Устав не заверен нотариально',
      }),
    )
  })
})

import { Link } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { toast } from 'sonner'
import { useAuth } from '@/hooks/useAuth'
import {
  type Beneficiary,
  type KycDocumentType,
  useDeleteDocument,
  useMyDocuments,
  useMyOrganization,
  useSubmitKyc,
  useUpdateBeneficiaries,
  useUploadDocument,
} from '@/hooks/useKyc'
import { validationMessage } from '@/i18n'
import { handleApiError } from '@/lib/api-errors'
import { kycStatusLabel, kycStatusVariant } from '@/lib/labels'
import { formatPercent } from '@/lib/money'
import { Button } from '@/components/ui/Button'
import { FieldError } from '@/components/ui/FieldError'
import { Input } from '@/components/ui/Input'
import { Badge } from '@/components/ui/Badge'
import { DocumentList } from '@/components/kyc/DocumentList'
import { DocumentFileLink } from '@/components/kyc/DocumentFileLink'

// Обязательные документы; кредитору бэкенд дополнительно требует source_of_funds.
// Подписи — kyc.documents.type.<тип> в словаре.
const REQUIRED_DOCS: KycDocumentType[] = ['charter', 'registration_certificate', 'address_proof', 'beneficiaries_register']
const LENDER_DOCS: KycDocumentType[] = ['source_of_funds']

const beneficiarySchema = z.object({
  full_name: z.string().trim().min(1, validationMessage('validation.fullName')),
  citizenship_country_code: z
    .string()
    .trim()
    .toUpperCase()
    .regex(/^[A-Z]{2}$/, validationMessage('validation.citizenship')),
  ownership_percent: z
    .string()
    .trim()
    .regex(/^\d{1,3}(\.\d{1,4})?$/, validationMessage('validation.shareFormat'))
    .refine((v) => Number(v) > 0 && Number(v) <= 100, validationMessage('validation.shareRange')),
})

export function KycStub() {
  const { t } = useTranslation()
  const { organization, user } = useAuth()

  if (!user) {
    return <div className="p-6">{t('kyc.loadingProfile')}</div>
  }

  if (!organization) {
    return (
      <div className="mx-auto max-w-xl p-6">
        <h1 className="text-2xl font-semibold">{t('kyc.noCompany.title')}</h1>
        <p className="mt-2 text-gray-600">
          {t('kyc.noCompany.text')}
        </p>
        <Link
          to="/profile"
          className="mt-4 inline-block rounded bg-blue-600 px-4 py-2 text-sm text-white"
        >
          {t('kyc.noCompany.action')}
        </Link>
      </div>
    )
  }

  const status = organization.kyc_status
  // Документы и бенефициаров бэкенд даёт менять только в draft / rejected.
  const editable = status === 'draft' || status === 'rejected'

  return (
    <div className="mx-auto max-w-2xl p-6">
      <h1 className="text-2xl font-semibold mb-2">{t('kyc.title')}</h1>

      <div className="mb-6 rounded-lg bg-white p-4 shadow border border-gray-200">
        <h2 className="text-lg font-medium text-gray-900">{organization.name}</h2>
        <div className="mt-2 text-sm text-gray-600 space-y-1">
          <p>{t('kyc.country', { value: organization.country_code })}</p>
          <p>{t('kyc.registrationNumber', { value: organization.registration_number })}</p>
          <p>{t('kyc.legalAddress', { value: organization.legal_address })}</p>
        </div>
        <div className="mt-4 flex items-center gap-2">
          <span className="text-sm font-medium text-gray-700">{t('kyc.status')}</span>
          <Badge variant={kycStatusVariant(status)}>{kycStatusLabel(status)}</Badge>
        </div>
      </div>

      {status === 'rejected' && (
        <div className="mb-6 rounded-lg border border-red-200 bg-red-50 p-4">
          <h3 className="font-medium text-red-800">{t('kyc.rejected.title')}</h3>
          {organization.kyc_reject_reason && (
            <p className="mt-1 text-sm text-red-700">{t('kyc.rejected.reason', { reason: organization.kyc_reject_reason })}</p>
          )}
          <p className="mt-2 text-sm text-red-700">{t('kyc.rejected.hint')}</p>
        </div>
      )}

      {editable && <KycForm isLender={organization.type === 'lender'} />}

      {status === 'under_review' && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 p-6 text-center">
          <h3 className="text-lg font-medium text-amber-800 mb-2">{t('kyc.underReview.title')}</h3>
          <p className="text-amber-700">
            {t('kyc.underReview.text')}
          </p>
        </div>
      )}

      {status === 'verified' && (
        <div className="rounded-lg border border-green-200 bg-green-50 p-6 text-center">
          <h3 className="text-lg font-medium text-green-800 mb-2">{t('kyc.verified.title')}</h3>
          <p className="text-green-700 mb-6">
            {t('kyc.verified.text')}
          </p>
          <Link
            to={user.role === 'borrower' ? '/applications' : '/offers'}
            className="inline-flex items-center rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-green-700"
          >
            {t('kyc.verified.action')}
          </Link>
        </div>
      )}

      {!editable && <SubmittedDocuments />}
    </div>
  )
}

// После отправки форма закрыта, но компания видит, что отправила на проверку.
function SubmittedDocuments() {
  const { t } = useTranslation()
  const { data: documents } = useMyDocuments()
  if (!documents) return null
  return (
    <div className="mt-6 rounded-lg border border-gray-200 bg-white p-4">
      <h3 className="text-lg font-medium text-gray-900 mb-4">{t('kyc.documents.submittedTitle')}</h3>
      <DocumentList documents={documents} />
    </div>
  )
}

function KycForm({ isLender }: { isLender: boolean }) {
  const { t } = useTranslation()
  const requiredDocs = isLender ? [...REQUIRED_DOCS, ...LENDER_DOCS] : REQUIRED_DOCS
  const { data: documents = [] } = useMyDocuments()
  const { data: profile } = useMyOrganization()
  const beneficiaries = profile?.beneficiaries ?? []
  const upload = useUploadDocument()
  const remove = useDeleteDocument()
  const saveBeneficiaries = useUpdateBeneficiaries()
  const submit = useSubmitKyc()

  const isAllUploaded = requiredDocs.every((type) => documents.some((d) => d.doc_type === type))
  const canSubmit = isAllUploaded && beneficiaries.length > 0

  const handleUpload = (docType: KycDocumentType, file: File | undefined) => {
    if (!file) return
    upload.mutate({ docType, file }, { onError: handleApiError })
  }

  const handleSubmit = () => {
    submit.mutate(undefined, {
      onSuccess: () => toast.success(t('kyc.submitted')),
      onError: handleApiError,
    })
  }

  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="text-lg font-medium text-gray-900 mb-4">{t('kyc.documents.title')}</h3>
        <p className="mb-3 text-xs text-gray-500">{t('kyc.documents.formats')}</p>
        <ul className="space-y-3">
          {requiredDocs.map((type) => {
            const uploaded = documents.find((d) => d.doc_type === type)
            return (
              <li key={type} className="flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between sm:gap-3">
                <span className="text-sm font-medium text-gray-700">{t(`kyc.documents.type.${type}`)}</span>
                {uploaded ? (
                  <span className="flex flex-wrap items-center gap-3 text-sm">
                    <DocumentFileLink doc={uploaded} className="min-w-0 break-all text-green-600 font-medium hover:underline">
                      ✓ {uploaded.file_name}
                    </DocumentFileLink>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={remove.isPending}
                      onClick={() => remove.mutate(uploaded.id, { onError: handleApiError })}
                    >
                      {t('common.delete')}
                    </Button>
                  </span>
                ) : (
                  <label className="cursor-pointer rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50">
                    {t('kyc.documents.upload')}
                    <input
                      type="file"
                      className="sr-only"
                      accept="application/pdf,image/png,image/jpeg"
                      disabled={upload.isPending}
                      onChange={(e) => {
                        handleUpload(type, e.target.files?.[0])
                        e.target.value = ''
                      }}
                    />
                  </label>
                )}
              </li>
            )
          })}
        </ul>
      </div>

      <BeneficiariesForm
        beneficiaries={beneficiaries}
        isSaving={saveBeneficiaries.isPending}
        onSave={(list, onSuccess) => saveBeneficiaries.mutate(list, { onSuccess, onError: handleApiError })}
      />

      <div className="flex justify-end">
        <Button onClick={handleSubmit} disabled={!canSubmit || submit.isPending}>
          {t('kyc.submit')}
        </Button>
      </div>
      {!canSubmit && (
        <p className="text-sm text-red-500 text-right mt-2">
          {t('kyc.submitBlocked')}
        </p>
      )}
    </div>
  )
}

type BeneficiaryForm = z.infer<typeof beneficiarySchema>

function BeneficiariesForm({
  beneficiaries,
  isSaving,
  onSave,
}: {
  beneficiaries: Beneficiary[]
  isSaving: boolean
  onSave: (list: Beneficiary[], onSuccess?: () => void) => void
}) {
  const { t } = useTranslation()
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<BeneficiaryForm>({ resolver: zodResolver(beneficiarySchema) })

  // PATCH заменяет список целиком, поэтому шлём его полностью.
  const onAdd = (b: BeneficiaryForm) => onSave([...beneficiaries, b], () => reset())

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4">
      <h3 className="text-lg font-medium text-gray-900 mb-4">{t('kyc.beneficiaries.title')}</h3>
      {beneficiaries.length > 0 ? (
        <ul className="mb-4 space-y-2">
          {beneficiaries.map((b, i) => (
            <li key={`${b.full_name}-${i}`} className="flex flex-col gap-2 rounded-md border p-3 text-sm sm:flex-row sm:items-center sm:justify-between sm:gap-0">
              <span className="min-w-0 break-words">
                {b.full_name} · {b.citizenship_country_code} · {formatPercent(b.ownership_percent)}
              </span>
              <Button
                size="sm"
                variant="outline"
                disabled={isSaving}
                onClick={() => onSave(beneficiaries.filter((_, j) => j !== i))}
              >
                {t('common.delete')}
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="mb-4 text-sm text-gray-500">{t('kyc.beneficiaries.empty')}</p>
      )}

      <form onSubmit={handleSubmit(onAdd)} className="grid gap-3 sm:grid-cols-[2fr_1fr_1fr_auto] sm:items-start">
        <div>
          <Input {...register('full_name')} placeholder={t('kyc.beneficiaries.fullName')} />
          <FieldError message={errors.full_name?.message} />
        </div>
        <div>
          <Input {...register('citizenship_country_code')} placeholder={t('kyc.beneficiaries.citizenship')} maxLength={2} />
          <FieldError message={errors.citizenship_country_code?.message} />
        </div>
        <div>
          <Input {...register('ownership_percent')} placeholder={t('kyc.beneficiaries.share')} inputMode="decimal" />
          <FieldError message={errors.ownership_percent?.message} />
        </div>
        <Button type="submit" variant="outline" disabled={isSaving}>
          {t('common.add')}
        </Button>
      </form>
    </div>
  )
}

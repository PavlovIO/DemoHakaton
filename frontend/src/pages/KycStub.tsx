import { Link } from 'react-router-dom'
import { useForm } from 'react-hook-form'
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
import { handleApiError } from '@/lib/api-errors'
import { formatPercent } from '@/lib/money'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Badge } from '@/components/ui/Badge'

// Обязательные документы; кредитору бэкенд дополнительно требует source_of_funds.
const REQUIRED_DOCS: { type: KycDocumentType; label: string }[] = [
  { type: 'charter', label: 'Устав компании' },
  { type: 'registration_certificate', label: 'Свидетельство о регистрации (ОГРН/ИНН)' },
  { type: 'address_proof', label: 'Подтверждение юридического адреса' },
  { type: 'beneficiaries_register', label: 'Сведения о бенефициарах' },
]
const LENDER_DOCS: { type: KycDocumentType; label: string }[] = [
  { type: 'source_of_funds', label: 'Подтверждение источника средств' },
]

const beneficiarySchema = z.object({
  full_name: z.string().trim().min(1, 'Укажите ФИО'),
  citizenship_country_code: z
    .string()
    .trim()
    .toUpperCase()
    .regex(/^[A-Z]{2}$/, 'Код страны из двух букв, например RU'),
  ownership_percent: z
    .string()
    .trim()
    .regex(/^\d{1,3}(\.\d{1,4})?$/, 'Доля в процентах, например 25 или 12.5')
    .refine((v) => Number(v) > 0 && Number(v) <= 100, 'Доля от 0 до 100%'),
})

export function KycStub() {
  const { organization, user } = useAuth()

  if (!user) {
    return <div className="p-6">Загрузка профиля...</div>
  }

  if (!organization) {
    return (
      <div className="mx-auto max-w-xl p-6">
        <h1 className="text-2xl font-semibold">Профиль компании</h1>
        <p className="mt-2 text-gray-600">
          У вашей учётной записи пока нет компании. Заполните профиль,
          чтобы получить доступ к заявкам и офферам.
        </p>
        <Link
          to="/profile"
          className="mt-4 inline-block rounded bg-blue-600 px-4 py-2 text-sm text-white"
        >
          Заполнить профиль
        </Link>
      </div>
    )
  }

  const status = organization.kyc_status
  // Документы и бенефициаров бэкенд даёт менять только в draft / rejected.
  const editable = status === 'draft' || status === 'rejected'

  return (
    <div className="mx-auto max-w-2xl p-6">
      <h1 className="text-2xl font-semibold mb-2">Верификация компании (KYC)</h1>

      <div className="mb-6 rounded-lg bg-white p-4 shadow border border-gray-200">
        <h2 className="text-lg font-medium text-gray-900">{organization.name}</h2>
        <div className="mt-2 text-sm text-gray-600 space-y-1">
          <p>Страна: {organization.country_code}</p>
          <p>Рег. номер: {organization.registration_number}</p>
          <p>Юр. адрес: {organization.legal_address}</p>
        </div>
        <div className="mt-4 flex items-center gap-2">
          <span className="text-sm font-medium text-gray-700">Статус:</span>
          {status === 'draft' && <Badge variant="default">Черновик</Badge>}
          {status === 'under_review' && <Badge variant="warning">На проверке</Badge>}
          {status === 'verified' && <Badge variant="success">Верифицирована</Badge>}
          {status === 'rejected' && <Badge variant="danger">Отклонена</Badge>}
        </div>
      </div>

      {status === 'rejected' && (
        <div className="mb-6 rounded-lg border border-red-200 bg-red-50 p-4">
          <h3 className="font-medium text-red-800">Верификация отклонена</h3>
          {organization.kyc_reject_reason && (
            <p className="mt-1 text-sm text-red-700">Причина: {organization.kyc_reject_reason}</p>
          )}
          <p className="mt-2 text-sm text-red-700">Исправьте документы и отправьте компанию на проверку повторно.</p>
        </div>
      )}

      {editable && <KycForm isLender={organization.type === 'lender'} />}

      {status === 'under_review' && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 p-6 text-center">
          <h3 className="text-lg font-medium text-amber-800 mb-2">Документы на проверке</h3>
          <p className="text-amber-700">
            Администратор проверит ваши документы в ближайшее время. Обычно это занимает до 24 часов.
          </p>
        </div>
      )}

      {status === 'verified' && (
        <div className="rounded-lg border border-green-200 bg-green-50 p-6 text-center">
          <h3 className="text-lg font-medium text-green-800 mb-2">Компания верифицирована!</h3>
          <p className="text-green-700 mb-6">
            Теперь вам доступны все функции платформы: создание заявок, предложений и сделок.
          </p>
          <Link
            to={user.role === 'borrower' ? '/applications' : '/offers'}
            className="inline-flex items-center rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-green-700"
          >
            Перейти к работе →
          </Link>
        </div>
      )}
    </div>
  )
}

function KycForm({ isLender }: { isLender: boolean }) {
  const requiredDocs = isLender ? [...REQUIRED_DOCS, ...LENDER_DOCS] : REQUIRED_DOCS
  const { data: documents = [] } = useMyDocuments()
  const { data: profile } = useMyOrganization()
  const beneficiaries = profile?.beneficiaries ?? []
  const upload = useUploadDocument()
  const remove = useDeleteDocument()
  const saveBeneficiaries = useUpdateBeneficiaries()
  const submit = useSubmitKyc()

  const isAllUploaded = requiredDocs.every((doc) => documents.some((d) => d.doc_type === doc.type))
  const canSubmit = isAllUploaded && beneficiaries.length > 0

  const handleUpload = (docType: KycDocumentType, file: File | undefined) => {
    if (!file) return
    upload.mutate({ docType, file }, { onError: handleApiError })
  }

  const handleSubmit = () => {
    submit.mutate(undefined, {
      onSuccess: () => toast.success('Документы отправлены на проверку администратору'),
      onError: handleApiError,
    })
  }

  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="text-lg font-medium text-gray-900 mb-4">Загрузка документов</h3>
        <p className="mb-3 text-xs text-gray-500">PDF, PNG или JPEG, до 10 МБ.</p>
        <ul className="space-y-3">
          {requiredDocs.map((doc) => {
            const uploaded = documents.find((d) => d.doc_type === doc.type)
            return (
              <li key={doc.type} className="flex items-center justify-between gap-3 p-3 border rounded-md">
                <span className="text-sm font-medium text-gray-700">{doc.label}</span>
                {uploaded ? (
                  <span className="flex items-center gap-3 text-sm">
                    <span className="text-green-600 font-medium">✓ {uploaded.file_name}</span>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={remove.isPending}
                      onClick={() => remove.mutate(uploaded.id, { onError: handleApiError })}
                    >
                      Удалить
                    </Button>
                  </span>
                ) : (
                  <label className="cursor-pointer rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50">
                    Загрузить файл
                    <input
                      type="file"
                      className="sr-only"
                      accept="application/pdf,image/png,image/jpeg"
                      disabled={upload.isPending}
                      onChange={(e) => {
                        handleUpload(doc.type, e.target.files?.[0])
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
          Отправить на проверку
        </Button>
      </div>
      {!canSubmit && (
        <p className="text-sm text-red-500 text-right mt-2">
          Загрузите все обязательные документы и укажите хотя бы одного бенефициара.
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
      <h3 className="text-lg font-medium text-gray-900 mb-4">Бенефициары</h3>
      {beneficiaries.length > 0 ? (
        <ul className="mb-4 space-y-2">
          {beneficiaries.map((b, i) => (
            <li key={`${b.full_name}-${i}`} className="flex items-center justify-between p-3 border rounded-md text-sm">
              <span>
                {b.full_name} · {b.citizenship_country_code} · {formatPercent(b.ownership_percent)}
              </span>
              <Button
                size="sm"
                variant="outline"
                disabled={isSaving}
                onClick={() => onSave(beneficiaries.filter((_, j) => j !== i))}
              >
                Удалить
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="mb-4 text-sm text-gray-500">Укажите хотя бы одного конечного бенефициара.</p>
      )}

      <form onSubmit={handleSubmit(onAdd)} className="grid gap-3 sm:grid-cols-[2fr_1fr_1fr_auto] sm:items-start">
        <div>
          <Input {...register('full_name')} placeholder="ФИО" />
          {errors.full_name && <p className="mt-1 text-xs text-red-600">{errors.full_name.message}</p>}
        </div>
        <div>
          <Input {...register('citizenship_country_code')} placeholder="Гражданство" maxLength={2} />
          {errors.citizenship_country_code && (
            <p className="mt-1 text-xs text-red-600">{errors.citizenship_country_code.message}</p>
          )}
        </div>
        <div>
          <Input {...register('ownership_percent')} placeholder="Доля, %" inputMode="decimal" />
          {errors.ownership_percent && <p className="mt-1 text-xs text-red-600">{errors.ownership_percent.message}</p>}
        </div>
        <Button type="submit" variant="outline" disabled={isSaving}>
          Добавить
        </Button>
      </form>
    </div>
  )
}

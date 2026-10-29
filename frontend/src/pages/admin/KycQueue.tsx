import { useState } from 'react'
import { toast } from 'sonner'
import { useApproveKyc, useKycQueue, useRejectKyc } from '@/hooks/useKyc'
import { handleApiError } from '@/lib/api-errors'
import { kycStatusLabel, roleLabel } from '@/lib/labels'
import type { components } from '@/lib/api-schema'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { LoadMore } from '@/components/ui/LoadMore'

type Organization = components['schemas']['Organization']

// Как AdminReason в api/schemas/errors.yaml.
const MIN_REASON_LENGTH = 3

interface RejectModalProps {
  org: Organization
  onClose: () => void
}

function RejectCompanyModal({ org, onClose }: RejectModalProps) {
  const rejectKyc = useRejectKyc()
  const [reason, setReason] = useState('')
  const trimmed = reason.trim()
  const canSubmit = trimmed.length >= MIN_REASON_LENGTH && !rejectKyc.isPending

  const handleSubmit = () => {
    if (!canSubmit) return
    rejectKyc.mutate(
      { id: org.id, reason: trimmed },
      {
        onSuccess: () => {
          toast.success('Компании отказано, она получит причину')
          onClose()
        },
        onError: handleApiError,
      },
    )
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
      <div className="w-full max-w-lg rounded-lg bg-white shadow-xl">
        <div className="border-b bg-gray-50 p-4 rounded-t-lg">
          <h3 className="text-lg font-semibold text-gray-900">Отказ в проверке компании</h3>
          <p className="mt-1 text-sm text-gray-500">{org.name} · {roleLabel(org.type)}</p>
        </div>
        <div className="p-4">
          <label htmlFor="reject-reason" className="mb-1 block text-sm font-medium text-gray-700">
            Причина (обязательно, минимум {MIN_REASON_LENGTH} символа). Компания увидит её в профиле.
          </label>
          <textarea
            id="reject-reason"
            rows={3}
            maxLength={2000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Например: устав без подписи директора, загрузите новую копию"
            className="w-full rounded border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-200"
          />
        </div>
        <div className="flex justify-end gap-2 border-t bg-gray-50 p-4 rounded-b-lg">
          <Button variant="outline" onClick={onClose}>Отмена</Button>
          <Button variant="danger" disabled={!canSubmit} onClick={handleSubmit}>
            {rejectKyc.isPending ? 'Отправка…' : 'Отказать'}
          </Button>
        </div>
      </div>
    </div>
  )
}

// Вкладка «Проверка компаний»: подтверждение одним кликом, отказ — через модалку с причиной.
export function KycQueueTab() {
  const kycQuery = useKycQueue()
  const { data: kyc, isLoading: isKycLoading } = kycQuery
  const kycQueue = kyc?.items ?? []
  const approveKyc = useApproveKyc()
  const [rejectingOrg, setRejectingOrg] = useState<Organization | null>(null)

  const handleApproveKyc = (id: string) => {
    approveKyc.mutate(id, {
      onSuccess: () => toast.success('Компания подтверждена'),
      onError: handleApiError,
    })
  }

  return (
    <div className="bg-white shadow overflow-hidden sm:rounded-md">
      <ul role="list" className="divide-y divide-gray-200">
        {kycQueue.map(org => (
          <li key={org.id} className="p-4 hover:bg-gray-50 flex items-center justify-between">
            <div>
              <div className="flex items-center gap-3 mb-1">
                <span className="font-semibold text-gray-900">{org.name}</span>
                <Badge variant="default">{roleLabel(org.type)}</Badge>
                <Badge variant="warning">{kycStatusLabel(org.kyc_status)}</Badge>
              </div>
              <p className="text-sm text-gray-500">
                Страна: {org.country_code} | Рег. номер: {org.registration_number}
              </p>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setRejectingOrg(org)}>Отклонить</Button>
              <Button disabled={approveKyc.isPending} onClick={() => handleApproveKyc(org.id)}>Подтвердить компанию</Button>
            </div>
          </li>
        ))}
        {!isKycLoading && kycQueue.length === 0 && <li className="p-6 text-center text-gray-500">Очередь пуста.</li>}
      </ul>
      <LoadMore query={kycQuery} />
      {rejectingOrg && <RejectCompanyModal org={rejectingOrg} onClose={() => setRejectingOrg(null)} />}
    </div>
  )
}

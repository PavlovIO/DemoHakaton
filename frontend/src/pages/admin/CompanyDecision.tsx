import { useState } from 'react'
import { toast } from 'sonner'
import { useApproveKyc, useRejectKyc } from '@/hooks/useKyc'
import { handleApiError } from '@/lib/api-errors'
import { roleLabel } from '@/lib/labels'
import type { components } from '@/lib/api-schema'
import { Button } from '@/components/ui/Button'

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

// Решение по проверке компании: подтверждение одним кликом, отказ — через модалку с причиной.
// Решать можно только по компании на проверке, в остальных статусах кнопок нет.
export function CompanyDecision({ company }: { company: Organization }) {
  const approveKyc = useApproveKyc()
  const [isRejecting, setIsRejecting] = useState(false)

  if (company.kyc_status !== 'under_review') return null

  const handleApprove = () => {
    approveKyc.mutate(company.id, {
      onSuccess: () => toast.success('Компания подтверждена'),
      onError: handleApiError,
    })
  }

  return (
    <div className="flex gap-2">
      <Button variant="outline" disabled={approveKyc.isPending} onClick={() => setIsRejecting(true)}>Отклонить</Button>
      <Button disabled={approveKyc.isPending} onClick={handleApprove}>Подтвердить компанию</Button>
      {isRejecting && <RejectCompanyModal org={company} onClose={() => setIsRejecting(false)} />}
    </div>
  )
}

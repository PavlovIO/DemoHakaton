import { toast } from 'sonner'
import { useApproveKyc, useKycQueue, useRejectKyc } from '@/hooks/useKyc'
import { handleApiError } from '@/lib/api-errors'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { LoadMore } from '@/components/ui/LoadMore'

// Вкладка «Очередь KYC» — перенесена из AdminPanel 1:1 (логика не менялась).
export function KycQueueTab() {
  const kycQuery = useKycQueue()
  const { data: kyc, isLoading: isKycLoading } = kycQuery
  const kycQueue = kyc?.items ?? []
  const approveKyc = useApproveKyc()
  const rejectKyc = useRejectKyc()

  const handleApproveKyc = (id: string) => {
    approveKyc.mutate(id, {
      onSuccess: () => toast.success('Компания верифицирована'),
      onError: handleApiError,
    })
  }

  const handleRejectKyc = (id: string) => {
    const reason = prompt('Причина отказа в верификации')?.trim()
    if (!reason) return
    rejectKyc.mutate({ id, reason }, {
      onSuccess: () => toast.success('Верификация отклонена, компания получит причину'),
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
                <Badge variant="default">{org.type === 'borrower' ? 'Заемщик' : 'Кредитор'}</Badge>
                <Badge variant="warning">На проверке</Badge>
              </div>
              <p className="text-sm text-gray-500">
                Страна: {org.country_code} | Рег. номер: {org.registration_number}
              </p>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" disabled={rejectKyc.isPending} onClick={() => handleRejectKyc(org.id)}>Отклонить</Button>
              <Button disabled={approveKyc.isPending} onClick={() => handleApproveKyc(org.id)}>Одобрить KYC</Button>
            </div>
          </li>
        ))}
        {!isKycLoading && kycQueue.length === 0 && <li className="p-6 text-center text-gray-500">Очередь пуста.</li>}
      </ul>
      <LoadMore query={kycQuery} />
    </div>
  )
}
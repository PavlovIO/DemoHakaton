import { useState } from 'react'
import { toast } from 'sonner'
import { useApproveApplication, useModerationQueue, useRejectApplication } from '@/hooks/useApplications'
import { useApproveKyc, useKycQueue, useRejectKyc } from '@/hooks/useKyc'
import { handleApiError } from '@/lib/api-errors'
import { formatMoney } from '@/lib/money'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { LoadMore } from '@/components/ui/LoadMore'

export function AdminPanel() {
  const [activeTab, setActiveTab] = useState<'kyc' | 'apps'>('kyc')
  
  const kycQuery = useKycQueue()
  const { data: kyc, isLoading: isKycLoading } = kycQuery
  const kycQueue = kyc?.items ?? []
  const approveKyc = useApproveKyc()
  const rejectKyc = useRejectKyc()
  const moderationQuery = useModerationQueue()
  const { data: moderation, isLoading: isModerationLoading } = moderationQuery
  const appQueue = moderation?.items ?? []
  const approveApplication = useApproveApplication()
  const rejectApplication = useRejectApplication()

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

  const handleApproveApp = (id: string) => {
    approveApplication.mutate(id, {
      onSuccess: () => toast.success('Заявка опубликована на витрине'),
      onError: handleApiError,
    })
  }

  const handleRejectApp = (id: string) => {
    const reason = prompt('Причина возврата на доработку')?.trim()
    if (!reason) return
    rejectApplication.mutate({ id, reason }, {
      onSuccess: () => toast.success('Заявка возвращена заемщику'),
      onError: handleApiError,
    })
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Панель администратора</h1>
        <p className="mt-1 text-sm text-gray-500">Управление верификацией и модерацией платформы</p>
      </div>

      {/* Табы */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex space-x-8">
          <button
            onClick={() => setActiveTab('kyc')}
            className={`whitespace-nowrap pb-4 px-1 border-b-2 font-medium text-sm ${
              activeTab === 'kyc' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
            }`}
          >
            Очередь KYC ({kyc?.total ?? 0})
          </button>
          <button
            onClick={() => setActiveTab('apps')}
            className={`whitespace-nowrap pb-4 px-1 border-b-2 font-medium text-sm ${
              activeTab === 'apps' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
            }`}
          >
            Модерация заявок ({moderation?.total ?? 0})
          </button>
        </nav>
      </div>

      {/* Вкладка KYC */}
      {activeTab === 'kyc' && (
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
      )}

      {/* Вкладка Заявок */}
      {activeTab === 'apps' && (
        <div className="bg-white shadow overflow-hidden sm:rounded-md">
          <ul role="list" className="divide-y divide-gray-200">
            {appQueue.map(app => (
              <li key={app.id} className="p-4 hover:bg-gray-50 flex items-center justify-between">
                <div>
                  <div className="flex items-center gap-3 mb-1">
                    <span className="font-mono text-sm text-gray-500">{app.number}</span>
                    <span className="font-semibold text-gray-900">{app.borrower.name}</span>
                    <Badge variant="warning">На модерации</Badge>
                  </div>
                  <p className="text-sm text-gray-900">{app.purpose}</p>
                  <p className="text-sm font-medium text-gray-700 mt-1">{formatMoney(app.amount, app.currency)}</p>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" disabled={rejectApplication.isPending} onClick={() => handleRejectApp(app.id)}>Отклонить</Button>
                  <Button disabled={approveApplication.isPending} onClick={() => handleApproveApp(app.id)}>Опубликовать</Button>
                </div>
              </li>
            ))}
            {!isModerationLoading && appQueue.length === 0 && <li className="p-6 text-center text-gray-500">Нет заявок на модерации.</li>}
          </ul>
          <LoadMore query={moderationQuery} />
        </div>
      )}
    </div>
  )
}
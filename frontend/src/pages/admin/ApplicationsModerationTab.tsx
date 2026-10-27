import { toast } from 'sonner'
import { useApproveApplication, useModerationQueue, useRejectApplication } from '@/hooks/useApplications'
import { handleApiError } from '@/lib/api-errors'
import { formatMoney } from '@/lib/money'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { LoadMore } from '@/components/ui/LoadMore'

// Вкладка «Модерация заявок» — перенесена из AdminPanel 1:1 (логика не менялась).
export function ApplicationsModerationTab() {
  const moderationQuery = useModerationQueue()
  const { data: moderation, isLoading: isModerationLoading } = moderationQuery
  const appQueue = moderation?.items ?? []
  const approveApplication = useApproveApplication()
  const rejectApplication = useRejectApplication()

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
  )
}

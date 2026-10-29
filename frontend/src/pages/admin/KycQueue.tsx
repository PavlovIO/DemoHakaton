import { Link } from 'react-router-dom'
import { useKycQueue } from '@/hooks/useKyc'
import { kycStatusLabel, roleLabel } from '@/lib/labels'
import { Badge } from '@/components/ui/Badge'
import { LoadMore } from '@/components/ui/LoadMore'
import { companyCardLink } from './paths'

// Вкладка «Проверка компаний» — очередь проверки. Кнопок решения в строках нет: решение
// принимается в карточке компании, рядом с её документами. Это правило интерфейса —
// бэкенд подтверждение без просмотра документов не запрещает.
export function KycQueueTab() {
  const kycQuery = useKycQueue()
  const { data: kyc, isLoading: isKycLoading } = kycQuery
  const kycQueue = kyc?.items ?? []

  return (
    <div className="bg-white shadow overflow-hidden sm:rounded-md">
      <ul role="list" className="divide-y divide-gray-200">
        {kycQueue.map(org => (
          <li key={org.id} className="p-4 hover:bg-gray-50">
            <div className="flex items-center gap-3 mb-1">
              <Link {...companyCardLink(org.id, 'kyc')} className="font-semibold text-blue-600 hover:underline">
                {org.name}
              </Link>
              <Badge variant="default">{roleLabel(org.type)}</Badge>
              <Badge variant="warning">{kycStatusLabel(org.kyc_status)}</Badge>
            </div>
            <p className="text-sm text-gray-500">
              Страна: {org.country_code} | Рег. номер: {org.registration_number}
            </p>
          </li>
        ))}
        {!isKycLoading && kycQueue.length === 0 && <li className="p-6 text-center text-gray-500">Очередь пуста.</li>}
      </ul>
      <LoadMore query={kycQuery} />
    </div>
  )
}

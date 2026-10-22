import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'

interface KycOrg {
  id: string
  name: string
  type: 'borrower' | 'lender'
  country_code: string
  registration_number: string
  status: 'under_review' | 'verified'
}

interface ModerationApp {
  id: string
  number: string
  company_name?: string
  amount: string
  currency: string
  purpose: string
  status: 'moderation' | 'published' | 'rejected' | string
}

const INITIAL_KYC: KycOrg[] = [
  { id: '1', name: 'Альфа', type: 'borrower', country_code: 'RU', registration_number: '7700000001', status: 'under_review' },
  { id: '2', name: 'Beta Corp', type: 'lender', country_code: 'CY', registration_number: 'CY-123456', status: 'under_review' },
  { id: '3', name: 'Омега', type: 'borrower', country_code: 'RU', registration_number: '7700000002', status: 'under_review' }
]

export function AdminPanel() {
  const [activeTab, setActiveTab] = useState<'kyc' | 'apps'>('kyc')
  
  const [kycQueue, setKycQueue] = useState<KycOrg[]>(INITIAL_KYC)
  const [appQueue, setAppQueue] = useState<ModerationApp[]>([])

  // Загружаем заявки из localStorage
  useEffect(() => {
    const savedApps = JSON.parse(localStorage.getItem('mock_applications') || '[]')
    setAppQueue(savedApps)
  }, [])

  const handleApproveKyc = (id: string) => {
    setKycQueue(kycQueue.map(org => org.id === id ? { ...org, status: 'verified' } : org))
    alert('Компания успешно верифицирована!')
  }

  const handleRejectKyc = (id: string) => {
    setKycQueue(kycQueue.filter(org => org.id !== id))
    alert('Верификация отклонена. Компания удалена из очереди.')
  }

  const handleApproveApp = (id: string) => {
    const updated = appQueue.map(app => app.id === id ? { ...app, status: 'published' as const } : app)
    setAppQueue(updated)
    localStorage.setItem('mock_applications', JSON.stringify(updated))
    alert('Заявка одобрена и опубликована на витрине!')
  }

  const handleRejectApp = (id: string) => {
    const updated = appQueue.map(app => app.id === id ? { ...app, status: 'rejected' as const } : app)
    setAppQueue(updated)
    localStorage.setItem('mock_applications', JSON.stringify(updated))
    alert('Заявка отклонена.')
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
            Очередь KYC ({kycQueue.filter(o => o.status === 'under_review').length})
          </button>
          <button
            onClick={() => setActiveTab('apps')}
            className={`whitespace-nowrap pb-4 px-1 border-b-2 font-medium text-sm ${
              activeTab === 'apps' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
            }`}
          >
            Модерация заявок ({appQueue.filter(a => a.status === 'moderation').length})
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
                    <Badge variant={org.status === 'under_review' ? 'warning' : 'success'}>
                      {org.status === 'under_review' ? 'На проверке' : 'Верифицирован'}
                    </Badge>
                  </div>
                  <p className="text-sm text-gray-500">
                    Страна: {org.country_code} | Рег. номер: {org.registration_number}
                  </p>
                </div>
                {org.status === 'under_review' && (
                  <div className="flex gap-2">
                    <Button variant="outline" onClick={() => handleRejectKyc(org.id)}>Отклонить</Button>
                    <Button onClick={() => handleApproveKyc(org.id)}>Одобрить KYC</Button>
                  </div>
                )}
              </li>
            ))}
            {kycQueue.length === 0 && <li className="p-6 text-center text-gray-500">Очередь пуста.</li>}
          </ul>
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
                    <span className="font-semibold text-gray-900">{app.company_name || 'Альфа'}</span>
                    <Badge variant={app.status === 'moderation' ? 'warning' : (app.status === 'published' ? 'success' : 'danger')}>
                      {app.status === 'moderation' ? 'На модерации' : (app.status === 'published' ? 'Опубликована' : 'Отклонена')}
                    </Badge>
                  </div>
                  <p className="text-sm text-gray-900">{app.purpose}</p>
                  <p className="text-sm font-medium text-gray-700 mt-1">{Number(app.amount).toLocaleString('ru-RU')} {app.currency}</p>
                </div>
                {app.status === 'moderation' && (
                  <div className="flex gap-2">
                    <Button variant="outline" onClick={() => handleRejectApp(app.id)}>Отклонить</Button>
                    <Button onClick={() => handleApproveApp(app.id)}>Опубликовать</Button>
                  </div>
                )}
              </li>
            ))}
            {appQueue.length === 0 && <li className="p-6 text-center text-gray-500">Нет заявок на модерации.</li>}
          </ul>
        </div>
      )}
    </div>
  )
}
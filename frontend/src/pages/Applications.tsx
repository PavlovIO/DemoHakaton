import { useState } from 'react'
import { useAuth } from '@/hooks/useAuth'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'

interface Application {
  id: string
  number: string
  amount: string
  currency: string
  target_country_code: string
  purpose: string
  term_months: number
  status: 'draft' | 'moderation' | 'published' | 'agreed' | 'rejected' | 'cancelled'
  created_at: string
}

const INITIAL_APPLICATIONS: Application[] = [
  {
    id: '1',
    number: 'APP-000001',
    amount: '500000.00',
    currency: 'EUR',
    target_country_code: 'DE',
    purpose: 'Закупка промышленного оборудования в Германии',
    term_months: 6,
    status: 'published',
    created_at: new Date().toISOString(),
  }
]

const getStatusBadge = (status: Application['status']) => {
  switch (status) {
    case 'draft': return <Badge variant="default">Черновик</Badge>
    case 'moderation': return <Badge variant="warning">На модерации</Badge>
    case 'published': return <Badge variant="success">Опубликована</Badge>
    case 'agreed': return <Badge variant="primary">Согласована</Badge>
    case 'rejected': return <Badge variant="danger">Отклонена</Badge>
    case 'cancelled': return <Badge variant="default">Отменена</Badge>
    default: return <Badge>{status}</Badge>
  }
}

export function Applications() {
  const { user } = useAuth()
  const [applications, setApplications] = useState<Application[]>(INITIAL_APPLICATIONS)
  const [isModalOpen, setIsModalOpen] = useState(false)

  // Состояние формы
  const [formData, setFormData] = useState({
    amount: '',
    currency: 'EUR',
    target_country_code: 'DE',
    purpose: '',
    term_months: '6'
  })

  if (user?.role !== 'borrower') {
    return <div className="p-6">Доступно только для заемщиков.</div>
  }

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    const newApp: Application = {
      id: Math.random().toString(),
      number: `APP-${Math.floor(Math.random() * 10000).toString().padStart(6, '0')}`,
      amount: formData.amount,
      currency: formData.currency,
      target_country_code: formData.target_country_code,
      purpose: formData.purpose,
      term_months: Number(formData.term_months),
      status: 'draft',
      created_at: new Date().toISOString(),
    }
    setApplications([newApp, ...applications])
    setIsModalOpen(false)
    setFormData({ amount: '', currency: 'EUR', target_country_code: 'DE', purpose: '', term_months: '6' })
  }

  return (
    <div className="space-y-6 relative">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold text-gray-900">Мои заявки</h1>
        <Button onClick={() => setIsModalOpen(true)}>+ Создать заявку</Button>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {applications.map((app) => (
            <li key={app.id} className="p-4 hover:bg-gray-50">
              <div className="flex items-center justify-between">
                <div>
                  <div className="flex items-center gap-3">
                    <span className="font-mono text-sm text-gray-500">{app.number}</span>
                    {getStatusBadge(app.status)}
                  </div>
                  <p className="mt-1 text-sm font-medium text-gray-900">{app.purpose}</p>
                  <div className="mt-1 text-sm text-gray-500 flex gap-4">
                    <span>{app.amount} {app.currency}</span>
                    <span>Срок: {app.term_months} мес.</span>
                    <span>Страна: {app.target_country_code}</span>
                  </div>
                </div>
              </div>
            </li>
          ))}
        </ul>
      </div>

      {/* Модальное окно создания заявки */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
            <h2 className="text-xl font-semibold mb-4">Создание заявки</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700">Сумма и валюта</label>
                <div className="mt-1 flex gap-2">
                  <input required type="number" min="1" step="0.01" value={formData.amount} onChange={e => setFormData({...formData, amount: e.target.value})} className="block w-full rounded-md border-gray-300 shadow-sm p-2 border" placeholder="500000.00" />
                  <select value={formData.currency} onChange={e => setFormData({...formData, currency: e.target.value})} className="rounded-md border-gray-300 shadow-sm p-2 border">
                    <option value="EUR">EUR</option>
                    <option value="USD">USD</option>
                    <option value="RUB">RUB</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Целевая юрисдикция</label>
                <select value={formData.target_country_code} onChange={e => setFormData({...formData, target_country_code: e.target.value})} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm p-2 border">
                  <option value="DE">Германия (DE)</option>
                  <option value="CN">Китай (CN)</option>
                  <option value="TR">Турция (TR)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Срок (месяцев)</label>
                <input required type="number" min="1" max="120" value={formData.term_months} onChange={e => setFormData({...formData, term_months: e.target.value})} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm p-2 border" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Назначение</label>
                <textarea required rows={3} value={formData.purpose} onChange={e => setFormData({...formData, purpose: e.target.value})} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm p-2 border" placeholder="Закупка оборудования..."></textarea>
              </div>
              <div className="flex justify-end gap-3 mt-6">
                <Button type="button" variant="outline" onClick={() => setIsModalOpen(false)}>Отмена</Button>
                <Button type="submit">Создать</Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
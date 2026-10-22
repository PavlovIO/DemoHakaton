import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'

interface Application {
  id: string
  number: string
  company_name: string
  amount: string
  currency: string
  target_country_code: string
  purpose: string
  term_months: number
  status: 'published'
}

// Моковые данные для витрины
const MOCK_MARKETPLACE: Application[] = [
  {
    id: '1',
    number: 'APP-000001',
    company_name: 'Альфа',
    amount: '500000.00',
    currency: 'EUR',
    target_country_code: 'DE',
    purpose: 'Закупка промышленного оборудования в Германии',
    term_months: 6,
    status: 'published',
  },
  {
    id: '2',
    number: 'APP-000003',
    company_name: 'Вектор',
    amount: '1200000.00',
    currency: 'USD',
    target_country_code: 'CN',
    purpose: 'Оплата контракта на поставку комплектующих',
    term_months: 12,
    status: 'published',
  },
  {
    id: '3',
    number: 'APP-000004',
    company_name: 'СтройИнвест',
    amount: '250000.00',
    currency: 'EUR',
    target_country_code: 'TR',
    purpose: 'Строительство логистического хаба',
    term_months: 9,
    status: 'published',
  }
]

export function Marketplace() {
  const [applications] = useState<Application[]>(MOCK_MARKETPLACE)
  
  // Состояние фильтров
  const [filterCountry, setFilterCountry] = useState('')
  const [filterCurrency, setFilterCurrency] = useState('')
  const [filterAmountMin, setFilterAmountMin] = useState('')

  // Логика фильтрации (на клиенте, так как бэкенд выключен)
  const filteredApps = applications.filter(app => {
    if (filterCountry && app.target_country_code !== filterCountry) return false
    if (filterCurrency && app.currency !== filterCurrency) return false
    if (filterAmountMin && Number(app.amount) < Number(filterAmountMin)) return false
    return true
  })

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Витрина заявок</h1>
        <p className="mt-1 text-sm text-gray-500">Опубликованные заявки на привлечение ликвидности</p>
      </div>

      {/* Панель фильтров */}
      <div className="bg-white p-4 rounded-lg shadow border border-gray-200 grid grid-cols-1 md:grid-cols-4 gap-4">
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">Страна</label>
          <select 
            value={filterCountry} 
            onChange={e => setFilterCountry(e.target.value)}
            className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm"
          >
            <option value="">Все страны</option>
            <option value="DE">Германия (DE)</option>
            <option value="CN">Китай (CN)</option>
            <option value="TR">Турция (TR)</option>
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">Валюта</label>
          <select 
            value={filterCurrency} 
            onChange={e => setFilterCurrency(e.target.value)}
            className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm"
          >
            <option value="">Любая</option>
            <option value="EUR">EUR</option>
            <option value="USD">USD</option>
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-700 mb-1">Сумма от</label>
          <input 
            type="number" 
            value={filterAmountMin}
            onChange={e => setFilterAmountMin(e.target.value)}
            placeholder="0"
            className="w-full rounded-md border-gray-300 shadow-sm p-2 border text-sm"
          />
        </div>
        <div className="flex items-end">
          <Button 
            variant="outline" 
            className="w-full" 
            onClick={() => { setFilterCountry(''); setFilterCurrency(''); setFilterAmountMin(''); }}
          >
            Сбросить
          </Button>
        </div>
      </div>

      {/* Список заявок */}
      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <ul role="list" className="divide-y divide-gray-200">
          {filteredApps.map((app) => (
            <li key={app.id} className="p-4 hover:bg-gray-50 transition-colors">
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div className="flex-1">
                  <div className="flex items-center gap-3 mb-1">
                    <span className="font-mono text-sm text-gray-500">{app.number}</span>
                    <Badge variant="success">Опубликована</Badge>
                    <span className="text-sm font-semibold text-gray-900">{app.company_name}</span>
                  </div>
                  <p className="text-sm text-gray-900">{app.purpose}</p>
                  <div className="mt-2 flex flex-wrap gap-4 text-sm text-gray-500">
                    <span className="font-medium text-gray-900">{Number(app.amount).toLocaleString('ru-RU')} {app.currency}</span>
                    <span>Срок: {app.term_months} мес.</span>
                    <span>Страна: {app.target_country_code}</span>
                  </div>
                </div>
                <div>
                  <Button onClick={() => alert('Здесь будет форма создания оффера (FE-07)')}>
                    Предложить финансирование
                  </Button>
                </div>
              </div>
            </li>
          ))}
          {filteredApps.length === 0 && (
            <li className="p-8 text-center text-gray-500">
              По вашему запросу ничего не найдено. Попробуйте изменить фильтры.
            </li>
          )}
        </ul>
      </div>
    </div>
  )
}
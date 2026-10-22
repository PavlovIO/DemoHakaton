import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '@/hooks/useAuth'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'

// Обязательные документы
const REQUIRED_DOCS = [
  { id: 'charter', label: 'Устав компании' },
  { id: 'registration_certificate', label: 'Свидетельство о регистрации (ОГРН/ИНН)' },
  { id: 'address_proof', label: 'Подтверждение юридического адреса' },
  { id: 'beneficiaries_register', label: 'Сведения о бенефициарах' },
]

export function KycStub() {
  const { organization, user, refresh } = useAuth()
  
  // Локальный стейт для имитации статуса и загруженных файлов
  const [localStatus, setLocalStatus] = useState(organization?.kyc_status || 'draft')
  const [uploadedDocs, setUploadedDocs] = useState<string[]>([])
  
  if (!organization || !user) {
    return <div className="p-6">Загрузка профиля...</div>
  }

  const handleMockUpload = (docId: string) => {
    if (!uploadedDocs.includes(docId)) {
      setUploadedDocs([...uploadedDocs, docId])
    }
  }

  const isAllUploaded = REQUIRED_DOCS.every(doc => uploadedDocs.includes(doc.id))

  const handleSubmit = () => {
    setLocalStatus('under_review')
    alert('Документы отправлены на проверку администратору!')
  }

  const handleSimulateApprove = () => {
    setLocalStatus('verified')
    alert('Администратор одобрил вашу компанию! Теперь вам доступны все функции.')
  }

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
          {localStatus === 'draft' && <Badge variant="default">Черновик</Badge>}
          {localStatus === 'under_review' && <Badge variant="warning">На проверке</Badge>}
          {localStatus === 'verified' && <Badge variant="success">Верифицирована</Badge>}
          {localStatus === 'rejected' && <Badge variant="danger">Отклонена</Badge>}
        </div>
      </div>

      {localStatus === 'draft' && (
        <div className="space-y-6">
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <h3 className="text-lg font-medium text-gray-900 mb-4">Загрузка документов</h3>
            <ul className="space-y-3">
              {REQUIRED_DOCS.map(doc => {
                const isUploaded = uploadedDocs.includes(doc.id)
                return (
                  <li key={doc.id} className="flex items-center justify-between p-3 border rounded-md">
                    <span className="text-sm font-medium text-gray-700">{doc.label}</span>
                    {isUploaded ? (
                      <span className="text-sm text-green-600 font-medium">✓ Загружен</span>
                    ) : (
                      <Button size="sm" variant="outline" onClick={() => handleMockUpload(doc.id)}>
                        Загрузить файл
                      </Button>
                    )}
                  </li>
                )
              })}
            </ul>
          </div>

          <div className="flex justify-end">
            <Button onClick={handleSubmit} disabled={!isAllUploaded}>
              Отправить на проверку
            </Button>
          </div>
          {!isAllUploaded && (
            <p className="text-sm text-red-500 text-right mt-2">
              Загрузите все обязательные документы, чтобы отправить заявку.
            </p>
          )}
        </div>
      )}

      {localStatus === 'under_review' && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 p-6 text-center">
          <h3 className="text-lg font-medium text-amber-800 mb-2">Документы на проверке</h3>
          <p className="text-amber-700 mb-6">
            Администратор проверит ваши документы в ближайшее время. Обычно это занимает до 24 часов.
          </p>
          <div className="border-t border-amber-200 pt-4 mt-4">
            <p className="text-xs text-amber-600 mb-2">Для демо-режима: симулируйте действия администратора</p>
            <Button onClick={handleSimulateApprove}>Симулировать одобрение админом</Button>
          </div>
        </div>
      )}

      {localStatus === 'verified' && (
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
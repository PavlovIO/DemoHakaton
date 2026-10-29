import { useTranslation } from 'react-i18next'
import type { KycDocument } from '@/hooks/useKyc'
import { currentLocale } from '@/i18n'
import { DocumentFileLink } from './DocumentFileLink'

function formatSize(bytes: number) {
  const unit = bytes < 1024 * 1024 ? 'kilobyte' : 'megabyte'
  const value = unit === 'kilobyte' ? bytes / 1024 : bytes / (1024 * 1024)
  return new Intl.NumberFormat(currentLocale(), { style: 'unit', unit, maximumFractionDigits: 1 }).format(value)
}

// Загруженные документы компании только для чтения: тип, файл со ссылкой на скачивание,
// размер и дата. Общий вид для компании (после отправки на проверку) и администратора.
export function DocumentList({ documents }: { documents: KycDocument[] }) {
  const { t } = useTranslation()

  if (documents.length === 0) {
    return <p className="text-sm text-gray-500">{t('kyc.documents.empty')}</p>
  }

  return (
    <ul className="divide-y divide-gray-200 rounded-md border border-gray-200">
      {documents.map((doc) => (
        <li key={doc.id} className="flex flex-wrap items-center justify-between gap-2 p-3 text-sm">
          <span className="font-medium text-gray-700">{t(`kyc.documents.type.${doc.doc_type}`)}</span>
          <span className="flex items-center gap-3">
            <DocumentFileLink doc={doc} className="text-blue-600 hover:underline" />
            <span className="text-gray-500">{formatSize(doc.size_bytes)}</span>
            <span className="text-gray-500">{new Date(doc.uploaded_at).toLocaleDateString(currentLocale())}</span>
          </span>
        </li>
      ))}
    </ul>
  )
}

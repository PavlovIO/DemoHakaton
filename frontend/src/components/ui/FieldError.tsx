import { useTranslation } from 'react-i18next'
import type { ValidationKey } from '@/i18n'

// message — ключ validation.* из схемы zod; незнакомую строку t вернёт как есть.
export function FieldError({ message }: { message?: string }) {
  const { t } = useTranslation()
  if (!message) return null
  return <p className="mt-1 text-xs text-red-600">{t(message as ValidationKey)}</p>
}

import { toast } from 'sonner'
import { i18n } from '@/i18n'
import type { Dictionary } from '@/i18n/ru'
import { ApiError } from './api-client'

// Человекочитаемые сообщения для кодов бэкенда — в словаре errors.<CODE>.
function errorMessage(code: string): string | undefined {
  const key = `errors.${code as keyof Dictionary['errors']}` as const
  return i18n.t(key, { defaultValue: '' }) || undefined
}

export function handleApiError(err: unknown): void {
  if (err instanceof ApiError) {
    const userMessage = errorMessage(err.code) ?? err.message ?? i18n.t('errors.unknown')
    toast.error(userMessage, {
      description: err.details ? formatDetails(err.details) : undefined,
      duration: 5000,
    })
    return
  }
  if (err instanceof Error) {
    toast.error(err.message || i18n.t('errors.network'))
    return
  }
  toast.error(i18n.t('errors.generic'))
}

function formatDetails(details: Record<string, string[]>): string {
  return Object.entries(details)
    .map(([field, msgs]) => `${field}: ${msgs.join(', ')}`)
    .join('\n')
}

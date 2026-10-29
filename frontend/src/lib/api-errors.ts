import { toast } from 'sonner'
import { ApiError } from './api-client'

// Человекочитаемые сообщения для кодов бэкенда
const ERROR_MESSAGES: Record<string, string> = {
  VALIDATION_ERROR: 'Проверьте правильность заполнения полей',
  UNAUTHORIZED: 'Требуется вход в систему',
  FORBIDDEN: 'У вас нет прав для этого действия',
  CSRF_INVALID: 'Сессия устарела. Обновите страницу и попробуйте снова',
  KYC_NOT_VERIFIED: 'Сначала пройдите проверку компании',
  NOT_FOUND: 'Запись не найдена',
  CONFLICT: 'Операция невозможна: данные уже изменились',
  INVALID_STATUS_TRANSITION: 'Переход в этот статус невозможен',
  RATE_LIMITED: 'Слишком много запросов. Подождите немного',
  INTERNAL: 'Внутренняя ошибка сервиса',
  NOT_IMPLEMENTED: 'Функция ещё в разработке',
  UNAVAILABLE: 'Сервис временно недоступен',
}

export function handleApiError(err: unknown): void {
  if (err instanceof ApiError) {
    const userMessage = ERROR_MESSAGES[err.code] ?? err.message ?? 'Неизвестная ошибка'
    toast.error(userMessage, {
      description: err.details ? formatDetails(err.details) : undefined,
      duration: 5000,
    })
    return
  }
  if (err instanceof Error) {
    toast.error(err.message || 'Ошибка сети')
    return
  }
  toast.error('Что-то пошло не так')
}

function formatDetails(details: Record<string, string[]>): string {
  return Object.entries(details)
    .map(([field, msgs]) => `${field}: ${msgs.join(', ')}`)
    .join('\n')
}
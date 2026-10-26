export function formatMoney(value: string | number | null | undefined, currency = 'RUB'): string {
  if (value === null || value === undefined || value === '') return '—'
  const num = typeof value === 'string' ? parseFloat(value) : value
  if (Number.isNaN(num)) return '—'
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(num)
}

export function formatPercent(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return '—'
  const num = typeof value === 'string' ? parseFloat(value) : value
  if (Number.isNaN(num)) return '—'
  return new Intl.NumberFormat('ru-RU', {
    style: 'percent',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(num / 100)
}

export function sanitizeDecimalInput(raw: string): string {
  return raw
    .replace(',', '.')
    .replace(/[^\d.]/g, '')
    .replace(/(\..*)\./g, '$1')
    .replace(/(\.\d{2})\d+/, '$1')
}
// Верхние пороги числовых полей (фронтенд-валидация, зеркалит ограничения бэкенда):
// проценты — не более 1_000_000 (%), сумма — не более 10^15 (NUMERIC(15,2) в БД).
export const MAX_PERCENT = 1_000_000
export const MAX_MONEY = 1_000_000_000_000_000 // 10^15

// Ограничивает число снизу и сверху; NaN/пустое возвращает null.
export function clampNumber(value: number | string | null | undefined, min = 0, max = MAX_MONEY): number | null {
  const num = typeof value === 'string' ? parseFloat(value) : value
  if (num === null || num === undefined || Number.isNaN(num)) return null
  return Math.min(Math.max(num, min), max)
}

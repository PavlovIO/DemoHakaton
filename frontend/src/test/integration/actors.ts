import { api } from '@/lib/api-client'
import { clearCookies, restoreCookies, snapshotCookies, type CookieSnapshot } from './browser-fetch'

const PASSWORD = 'Demo-Pass-2026'

// Верифицированные участники демо-сида: заявки и офферы доступны только им.
export const BORROWER = 'vector@liquidity.local'
export const LENDER = 'epsilon@liquidity.local'
export const ADMIN = 'admin@liquidity.local'

// Сессии участников на файл тестов. Лимит логина — 50 с IP за 15 минут,
// поэтому каждый участник входит один раз, а сценарии переключают cookie.
const sessions = new Map<string, CookieSnapshot>()

export async function loginAs(email: string) {
  const saved = sessions.get(email)
  if (saved) {
    restoreCookies(saved)
    return
  }
  clearCookies()
  await api.post('/auth/login', { email, password: PASSWORD })
  sessions.set(email, snapshotCookies())
}

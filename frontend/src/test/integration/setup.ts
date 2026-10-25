import { afterEach, beforeAll } from 'vitest'
import { cleanup } from '@testing-library/react'
import { API_URL, clearCookies, installBrowserFetch } from './browser-fetch'

beforeAll(async () => {
  const res = await fetch(`${API_URL}/api/v1/healthz`).catch(() => null)
  if (!res?.ok) {
    throw new Error(`Бэкенд недоступен на ${API_URL}: make db && make backend-up`)
  }
  installBrowserFetch()
})

afterEach(() => {
  cleanup()
  clearCookies()
})

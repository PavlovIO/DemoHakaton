/// <reference types="node" />
// Fetch как в браузере за Vite proxy: относительные /api/... уходят на живой бэкенд,
// cookie хранятся в jar и отправляются обратно, не-HttpOnly (csrf_token) видны в document.cookie.
// Node fetch cookie не хранит, а jsdom не даёт поставить HttpOnly через document.cookie.

export const API_URL = process.env.INTEGRATION_API_URL ?? 'http://localhost:8080'

interface StoredCookie {
  value: string
  httpOnly: boolean
}

const jar = new Map<string, StoredCookie>()
const nodeFetch = globalThis.fetch

function storeSetCookie(header: string) {
  const [pair, ...attrs] = header.split(';').map((s) => s.trim())
  const eq = pair.indexOf('=')
  const name = pair.slice(0, eq)
  const value = pair.slice(eq + 1)
  const lower = attrs.map((a) => a.toLowerCase())
  const httpOnly = lower.includes('httponly')
  const expired =
    value === '' ||
    lower.some((a) => a === 'max-age=0' || a.startsWith('max-age=-')) ||
    lower.some((a) => a.startsWith('expires=') && Date.parse(a.slice(8)) <= Date.now())

  if (expired) {
    jar.delete(name)
    if (!httpOnly) document.cookie = `${name}=; Max-Age=0; Path=/`
    return
  }
  jar.set(name, { value, httpOnly })
  if (!httpOnly) document.cookie = `${name}=${value}; Path=/`
}

export function installBrowserFetch() {
  globalThis.fetch = async (input, init) => {
    const url = typeof input === 'string' && input.startsWith('/') ? API_URL + input : input
    const headers = new Headers(init?.headers)
    if (jar.size > 0) {
      headers.set('Cookie', [...jar].map(([name, c]) => `${name}=${c.value}`).join('; '))
    }
    const res = await nodeFetch(url, { ...init, headers })
    res.headers.getSetCookie().forEach(storeSetCookie)
    return res
  }
}

// Новый «браузер» для каждого теста: без сессии и CSRF-cookie.
export function clearCookies() {
  for (const [name, c] of jar) {
    if (!c.httpOnly) document.cookie = `${name}=; Max-Age=0; Path=/`
  }
  jar.clear()
}

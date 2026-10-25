import type { paths, components } from './api-schema'

export type ApiErrorBody = components['schemas']['Error']

export class ApiError extends Error {
  status: number
  code: string
  details?: Record<string, string[]>

  constructor(status: number, body: ApiErrorBody) {
    super(body.error?.message ?? body.error?.code ?? 'Неизвестная ошибка')
    this.name = 'ApiError'
    this.status = status
    this.code = body.error?.code ?? 'UNKNOWN'

    const details: Record<string, string[]> = {}

    for (const { field, message } of body.error?.details ?? []) {
      details[field] ??= []
      details[field].push(message)
    }

    this.details = Object.keys(details).length > 0 ? details : undefined
  }
}

// Читаем cookie по имени (csrf_token не HttpOnly — фронт может его прочитать)
function getCookie(name: string): string | null {
  const match = document.cookie.match(new RegExp('(?:^|;\\s*)' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : null
}

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

interface RequestOptions {
  method?: Method
  body?: unknown
  query?: Record<string, string | number | boolean | undefined | null | Array<string | number>>
  // Значения для {param} в шаблоне пути из OpenAPI: '/offers/{offerId}/accept', { path: { offerId } }
  path?: Record<string, string>
  signal?: AbortSignal
}

function buildPath(template: string, params: Record<string, string> = {}): string {
  return template.replace(/\{(\w+)\}/g, (_, name: string) => {
    const value = params[name]
    if (value === undefined) throw new Error(`Не задан параметр пути ${name} для ${template}`)
    return encodeURIComponent(value)
  })
}

// Сериализация query-параметров (включая массивы в формате ?status=sent&status=accepted)
function buildQuery(query?: RequestOptions['query']): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null) continue
    if (Array.isArray(v)) {
      v.forEach((item) => params.append(k, String(item)))
    } else {
      params.append(k, String(v))
    }
  }
  const s = params.toString()
  return s ? `?${s}` : ''
}

async function request<T>(path: keyof paths | string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, query, path: params, signal } = options
  const url = `/api/v1${buildPath(String(path), params)}${buildQuery(query)}`

  const headers: Record<string, string> = {
    Accept: 'application/json',
  }

  // CSRF: для всех небезопасных методов добавляем X-CSRF-Token из cookie
  const unsafeMethods: Method[] = ['POST', 'PUT', 'PATCH', 'DELETE']
  if (unsafeMethods.includes(method)) {
    const csrf = getCookie('csrf_token')
    if (csrf) headers['X-CSRF-Token'] = csrf
  }

  // FormData (загрузка файлов) уходит как есть: boundary в Content-Type ставит fetch.
  const isForm = body instanceof FormData
  if (body !== undefined && !isForm) headers['Content-Type'] = 'application/json'

  const res = await fetch(url, {
    method,
    headers,
    credentials: 'include', // обязательно: шлём и принимаем cookie
    body: body === undefined ? undefined : isForm ? body : JSON.stringify(body),
    signal,
  })

  // 204 No Content (например, logout)
  if (res.status === 204) return undefined as T

  const data = await res.json().catch(() => null)

  if (!res.ok) {
    // 401 — сессия истекла, на логин
    if (res.status === 401) {
      if (!window.location.pathname.startsWith('/login')) {
        window.location.href = '/login'
      }
    }

    if (data && typeof data === 'object' && 'error' in data) {
      throw new ApiError(res.status, data as ApiErrorBody)
    }
    throw new ApiError(res.status, {error: {
      code: 'INTERNAL',
      message: `HTTP ${res.status}`,
    }})
  }

  return data as T
}

// Удобные обёртки с типизацией через OpenAPI
export const api = {
  get: <T>(path: keyof paths, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...opts, method: 'GET' }),

  post: <T>(path: keyof paths, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...opts, method: 'POST', body }),

  patch: <T>(path: keyof paths, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...opts, method: 'PATCH', body }),

  delete: <T>(path: keyof paths, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...opts, method: 'DELETE' }),
}
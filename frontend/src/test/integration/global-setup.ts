/// <reference types="node" />
import type { TestProject } from 'vitest/node'
import { ACTORS, PASSWORD, type SerializedSession } from './actors-config'

const API_URL = process.env.INTEGRATION_API_URL ?? 'http://localhost:8080'

// Участники входят один раз за прогон, а не в каждом файле: лимит логина —
// 50 с IP за 15 минут, и повторные локальные прогоны иначе упираются в 429.
export default async function setup(project: TestProject) {
  const health = await fetch(`${API_URL}/api/v1/healthz`).catch(() => null)
  if (!health?.ok) return // понятную ошибку покажет setup.ts

  const sessions: Record<string, SerializedSession> = {}
  for (const email of ACTORS) {
    const res = await fetch(`${API_URL}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password: PASSWORD }),
    })
    if (!res.ok) throw new Error(`Вход ${email}: HTTP ${res.status}`)
    sessions[email] = res.headers.getSetCookie().map((header) => {
      const [pair, ...attrs] = header.split(';').map((s) => s.trim())
      const eq = pair.indexOf('=')
      return {
        name: pair.slice(0, eq),
        value: pair.slice(eq + 1),
        httpOnly: attrs.some((a) => a.toLowerCase() === 'httponly'),
      }
    })
  }
  project.provide('sessions', sessions)
}

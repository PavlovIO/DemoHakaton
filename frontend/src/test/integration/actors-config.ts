export const PASSWORD = 'Demo-Pass-2026'

// Верифицированные участники демо-сида: заявки и офферы доступны только им.
export const BORROWER = 'vector@liquidity.local'
export const LENDER = 'epsilon@liquidity.local'
export const ADMIN = 'admin@liquidity.local'

export const ACTORS = [BORROWER, LENDER, ADMIN] as const

export type SerializedSession = { name: string; value: string; httpOnly: boolean }[]

declare module 'vitest' {
  export interface ProvidedContext {
    sessions: Record<string, SerializedSession>
  }
}

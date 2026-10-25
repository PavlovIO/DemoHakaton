import { inject } from 'vitest'
import { restoreCookies } from './browser-fetch'

export { ADMIN, BORROWER, LENDER } from './actors-config'

// Переключить общий «браузер» на сессию участника, открытую в global-setup.
export function loginAs(email: string) {
  const session = inject('sessions')[email]
  if (!session) throw new Error(`Нет сессии для ${email}: участник не в ACTORS`)
  restoreCookies(new Map(session.map(({ name, ...cookie }) => [name, cookie])))
}

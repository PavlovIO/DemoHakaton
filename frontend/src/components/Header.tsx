import { Navigation } from './Navigation'
import { NotificationsDropdown } from './NotificationsDropdown'
import { ProfileDropdown } from './ProfileDropdown'
import { LanguageSwitcher } from './LanguageSwitcher'
import { useAuth } from '@/hooks/useAuth'
import { canChooseLanguage } from '@/i18n'

export function Header() {
  const { user } = useAuth()

  return (
    <header className="sticky top-0 z-40 border-b border-green-300 bg-chrome">
      <div className="mx-auto flex max-w-7xl flex-col gap-1 py-2 sm:py-2 lg:h-16 lg:flex-row lg:items-center lg:justify-between lg:gap-8 lg:py-0 px-4 sm:px-6 lg:px-8">
        <div className="flex min-w-0 flex-1 items-center justify-between gap-4 lg:justify-between sm:gap-8">
          <a href="/dashboard" className="flex shrink-0 items-center gap-2">
            <svg className="h-8 w-8" viewBox="0 0 64 64" aria-hidden="true">
              <rect width="64" height="64" rx="15" fill="#89cfad" />
              <path d="M20 16 V46 M44 16 V46" stroke="#fff" strokeWidth="4" strokeLinecap="round" />
              <path
                d="M20 16 Q32 40 44 16 M8 34 L20 16 M44 16 L56 34"
                fill="none"
                stroke="#fff"
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
              <path d="M8 40 H56" stroke="#fff" strokeWidth="4.5" strokeLinecap="round" />
            </svg>
            <span className="hidden truncate text-lg font-bold text-gray-900 sm:block">Liquidity Platform</span>
          </a>
          <div className="flex shrink-0 items-center gap-2 lg:hidden">
            {canChooseLanguage(user?.role) && <LanguageSwitcher />}
            <NotificationsDropdown />
            <ProfileDropdown />
          </div>

          <div className="hidden min-w-0 flex-1 lg:flex">
            <Navigation />
          </div>
          
        </div>

        <div className="hidden shrink-0 items-center gap-2 lg:flex">
          {/* Английский — только кредиторам (#102), см. canChooseLanguage. */}
          {canChooseLanguage(user?.role) && <LanguageSwitcher className="mr-2" />}
          <NotificationsDropdown />
          <ProfileDropdown />
        </div>
        <div className="-mx-4 overflow-x-auto px-4 pb-1 lg:hidden">
          <Navigation />
        </div>
      </div>
    </header>
  )
}

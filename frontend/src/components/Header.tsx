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
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
        <div className="flex items-center gap-8">
          <a href="/dashboard" className="flex items-center gap-2">
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
            <span className="text-lg font-bold text-gray-900">Liquidity Platform</span>
          </a>

          <Navigation />
        </div>

        <div className="flex items-center gap-2">
          {/* Английский — только кредиторам (#102), см. canChooseLanguage. */}
          {canChooseLanguage(user?.role) && <LanguageSwitcher className="mr-2" />}
          <NotificationsDropdown />
          <ProfileDropdown />
        </div>
      </div>
    </header>
  )
}
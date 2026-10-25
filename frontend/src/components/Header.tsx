import { Navigation } from './Navigation'
import { NotificationsDropdown } from './NotificationsDropdown'
import { ProfileDropdown } from './ProfileDropdown'
import { Breadcrumbs } from './Breadcrumbs'

export function Header() {
  return (
    <header className="sticky top-0 z-40 border-b border-gray-200 bg-white">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
        <div className="flex items-center gap-8">
          <a href="/dashboard" className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary-600 text-white">
              <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M12 2L2 7l10 5 10-5-10-5z" />
                <path d="M2 17l10 5 10-5" />
                <path d="M2 12l10 5 10-5" />
              </svg>
            </div>
            <span className="text-lg font-bold text-gray-900">Liquidity Platform</span>
          </a>

          <Navigation />
        </div>

        <div className="flex items-center gap-2">
          <NotificationsDropdown />
          <ProfileDropdown />
        </div>
      </div>

      <div className="mx-auto max-w-7xl px-4 py-2 sm:px-6 lg:px-8">
        <Breadcrumbs />
      </div>
    </header>
  )
}
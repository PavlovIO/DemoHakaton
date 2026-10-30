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
      {/* Два уровня на мобильных: верхняя строка — логотип и профиль, ниже — навигация во всю ширину.
          На lg+ навигация встаёт в одну строку с логотипом (lg:flex-row). */}
      <div className="mx-auto flex max-w-7xl flex-col gap-1 py-2 sm:py-2 lg:h-16 lg:flex-row lg:items-center lg:justify-between lg:gap-8 lg:py-0 px-4 sm:px-6 lg:px-8">
        <div className="flex min-w-0 flex-1 items-center justify-between gap-4 lg:justify-start sm:gap-8">
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

          {/* На мобильных навигация уходит на второй уровень (вниз), здесь остаётся только правый блок. */}
          <div className="flex shrink-0 items-center gap-2 lg:hidden">
            {/* Английский — только кредиторам (#102), см. canChooseLanguage. */}
            {canChooseLanguage(user?.role) && <LanguageSwitcher />}
            <NotificationsDropdown />
            <ProfileDropdown />
          </div>

          <div className="hidden min-w-0 flex-1 lg:block">
            <Navigation />
          </div>
        </div>

        {/* Верхний уровень на десктопе: правые элементы в той же строке; на мобильных скрыт (дубликат). */}
        <div className="hidden shrink-0 items-center gap-2 lg:flex">
          {canChooseLanguage(user?.role) && <LanguageSwitcher className="mr-2" />}
          <NotificationsDropdown />
          <ProfileDropdown />
        </div>

        {/* Второй уровень на мобильных: навигация во всю ширину. overflow-x-auto — подстраховка
            для роли админа с 6 пунктами; у заемщика/кредитора всё помещается без скролла. */}
        <div className="-mx-4 overflow-x-auto px-4 pb-1 lg:hidden">
          <Navigation />
        </div>
      </div>
    </header>
  )
}
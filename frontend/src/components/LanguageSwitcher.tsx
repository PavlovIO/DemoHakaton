import { Globe } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { LANGUAGES, currentLanguage, setLanguage } from '@/i18n'
import { cn } from '@/lib/utils'

// Подписи кодом языка, а не названием: «EN» узнает и тот, кто не читает по-русски.
export function LanguageSwitcher({ className }: { className?: string }) {
  const { t } = useTranslation()
  const active = currentLanguage()

  return (
    <div role="group" aria-label={t('language.label')} className={cn('flex items-center gap-1 text-sm', className)}>
      <Globe className="h-4 w-4 text-gray-500" aria-hidden="true" />
      {LANGUAGES.map((language, i) => (
        <span key={language} className="flex items-center gap-1">
          {i > 0 && <span className="text-gray-300" aria-hidden="true">|</span>}
          <button
            type="button"
            aria-pressed={language === active}
            onClick={() => setLanguage(language)}
            className={cn(
              'rounded px-1 py-0.5 font-medium uppercase',
              language === active ? 'text-gray-900' : 'text-gray-500 hover:text-gray-900',
            )}
          >
            {language}
          </button>
        </span>
      ))}
    </div>
  )
}

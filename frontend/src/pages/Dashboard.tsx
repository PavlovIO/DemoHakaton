import { useTranslation } from 'react-i18next'

export function Dashboard() {
  const { t } = useTranslation()

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">{t('dashboard.title')}</h1>
      <p className="mt-2 text-gray-600">{t('dashboard.welcome')}</p>
    </div>
  )
}
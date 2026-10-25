import { Link } from 'react-router-dom'
import { useAuth, type KycStatus } from '@/hooks/useAuth'

const STATUS_TEXT: Record<KycStatus, string> = {
  draft: 'Черновик — загрузите документы и отправьте на проверку',
  under_review: 'Документы на проверке у администратора',
  verified: 'Компания верифицирована',
  rejected: 'Отклонено — внесите правки и подайте повторно',
}

export function KycStub() {
  const { organization } = useAuth()

  if (!organization) {
    return (
      <div className="mx-auto max-w-xl p-6">
        <h1 className="text-2xl font-semibold">Профиль компании</h1>
        <p className="mt-2 text-gray-600">
          У вашей учётной записи пока нет компании. Заполните профиль,
          чтобы получить доступ к заявкам и офферам.
        </p>
        <Link
          to="/profile"
          className="mt-4 inline-block rounded bg-blue-600 px-4 py-2 text-sm text-white"
        >
          Заполнить профиль
        </Link>
      </div>
    )
  }

  const status = organization.kyc_status

  return (
    <div className="mx-auto max-w-xl p-6">
      <h1 className="text-2xl font-semibold">Верификация компании (KYC)</h1>
      <p className="mt-2 text-gray-600">
        {organization.name}
      </p>

      <div className="mt-2 flex items-center gap-2">
        <span className="rounded bg-gray-100 px-2 py-1 font-mono text-xs">
          {status}
        </span>
        <span className="text-sm text-gray-600">
          {STATUS_TEXT[status] ?? 'Неизвестный статус'}
        </span>
      </div>

      {status === 'verified' ? (
        <Link to="/applications" className="mt-4 inline-block text-blue-600">
          Перейти к заявкам →
        </Link>
      ) : (
        <div className="mt-4 rounded border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
          Загрузка документов и отправка на проверку реализуются в Фазе 2.
        </div>
      )}
    </div>
  )
}
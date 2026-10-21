import { Link } from 'react-router-dom'
import { useAuth } from '@/hooks/useAuth'

export function ProfileStub() {
  const { user, organization } = useAuth()

  if (organization) {
    return (
      <div className="mx-auto max-w-xl p-6">
        <h1 className="text-2xl font-semibold">Профиль компании</h1>
        <p className="mt-2 text-gray-600">
          Профиль уже заполнен. Перейдите к верификации.
        </p>
        <Link
          to="/profile/kyc"
          className="mt-4 inline-block rounded bg-blue-600 px-4 py-2 text-sm text-white"
        >
          Перейти к KYC →
        </Link>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-xl p-6">
      <h1 className="text-2xl font-semibold">Профиль компании</h1>

      {user && (
        <p className="mt-2 text-gray-600">
          Пользователь: <span className="font-medium">{user.email}</span>
          <br />
          Роль: <span className="font-medium">{user.role}</span>
        </p>
      )}

      <div className="mt-4 rounded border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800">
        <p className="font-medium">Профиль компании не заполнен</p>
        <p className="mt-1">
          Для работы с заявками и офферами необходимо заполнить профиль компании
          и пройти верификацию.
        </p>
      </div>

      <div className="mt-6 rounded border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
        <p className="font-medium">Что будет реализовано в Фазе 2:</p>
        <ul className="mt-2 list-inside list-disc space-y-1">
          <li>Форма редактирования профиля компании</li>
          <li>Загрузка учредительных документов</li>
          <li>Отправка на верификацию администратору</li>
        </ul>
      </div>

      <Link
        to="/dashboard"
        className="mt-6 inline-block text-sm text-gray-600 hover:text-gray-900"
      >
        Вернуться на главную
      </Link>
    </div>
  )
}
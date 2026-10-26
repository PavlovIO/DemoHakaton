import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { LogOut, Building, ShieldCheck, ShieldAlert } from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'

const ROLE_LABEL: Record<string, string> = {
  borrower: 'Заёмщик',
  lender: 'Кредитор',
  admin: 'Администратор',
}

const KYC_LABEL: Record<string, { text: string; className: string }> = {
  draft: { text: 'Не начато', className: 'bg-gray-100 text-gray-700' },
  under_review: { text: 'На проверке', className: 'bg-amber-100 text-amber-800' },
  verified: { text: 'Подтверждено', className: 'bg-emerald-100 text-emerald-800' },
  rejected: { text: 'Отклонено', className: 'bg-red-100 text-red-800' },
}

export function ProfileDropdown() {
  const [isOpen, setIsOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const { me, logout } = useAuth()

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  useEffect(() => {
    if (!isOpen) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setIsOpen(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [isOpen])

  if (!me) return null

  const { user, organization } = me

  // У админа организации нет (Me.organization = null) — без компании и KYC.
  const initials = (organization?.name ?? user.email)
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2)

  const kyc = organization ? (KYC_LABEL[organization.kyc_status] ?? KYC_LABEL.draft) : null
  const isKycVerified = organization?.kyc_status === 'verified'

  const go = (path: string) => {
    setIsOpen(false)
    navigate(path)
  }

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        type="button"
        onClick={() => setIsOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        className="flex items-center gap-2 rounded-md p-1.5 hover:bg-gray-100"
      >
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 text-sm font-semibold text-primary-700">
          {initials || '?'}
        </div>
      </button>

      {isOpen && (
        <div
          role="menu"
          className="absolute right-0 z-50 mt-2 w-72 rounded-lg border border-gray-200 bg-white shadow-lg"
        >
          {/* Шапка */}
          <div className="border-b border-gray-200 px-4 py-3">
            {organization && (
              <p className="truncate text-sm font-medium text-gray-900">
                {organization.name}
              </p>
            )}
            <p className="truncate text-xs text-gray-500">{user.email}</p>

            <div className="mt-2 flex flex-wrap gap-1.5">
              <span className="inline-flex items-center rounded-full bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700">
                {ROLE_LABEL[user.role] ?? user.role}
              </span>
              {kyc && (
                <span
                  className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${kyc.className}`}
                >
                  {isKycVerified ? (
                    <ShieldCheck className="h-3 w-3" />
                  ) : (
                    <ShieldAlert className="h-3 w-3" />
                  )}
                  {kyc.text}
                </span>
              )}
            </div>
          </div>

          {/* Пункты меню */}
          <div className="py-1">
            {organization && (
              <button
                type="button"
                role="menuitem"
                onClick={() => go('/profile')}
                className="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100"
              >
                <Building className="h-4 w-4" />
                Профиль компании
              </button>
            )}

            {/* Если KYC не пройден — подсвечиваем пункт, ведущий к верификации */}
            {organization && !isKycVerified && (
              <button
                type="button"
                role="menuitem"
                onClick={() => go('/profile/kyc')}
                className="flex w-full items-center gap-2 px-4 py-2 text-left text-sm font-medium text-amber-700 hover:bg-amber-50"
              >
                <ShieldAlert className="h-4 w-4" />
                Пройти верификацию (KYC)
              </button>
            )}
          </div>

          {/* Выход */}
          <div className="border-t border-gray-200 py-1">
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                setIsOpen(false)
                void logout()
              }}
              className="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-danger hover:bg-gray-100"
            >
              <LogOut className="h-4 w-4" />
              Выйти
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
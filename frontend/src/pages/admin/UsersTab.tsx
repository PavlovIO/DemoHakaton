import { useState } from 'react'
import { toast } from 'sonner'
import { useAuth } from '@/hooks/useAuth'
import { useDebounce } from '@/hooks/useDebounce'
import { useAdminUsers, useBlockUser, useUnblockUser, type AdminUser } from '@/hooks/useAdmin'
import { handleApiError } from '@/lib/api-errors'
import { roleLabel, userStatusLabel } from '@/lib/labels'
import type { components } from '@/lib/api-schema'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { LoadMore } from '@/components/ui/LoadMore'
import { currentLocale } from '@/i18n'

type Schemas = components['schemas']

const ROLE_OPTIONS: { value: '' | Schemas['Role']; label: string }[] = [
  { value: '', label: 'Все роли' },
  { value: 'admin', label: roleLabel('admin') },
  { value: 'borrower', label: roleLabel('borrower') },
  { value: 'lender', label: roleLabel('lender') },
]

const STATUS_OPTIONS: { value: '' | Schemas['UserStatus']; label: string }[] = [
  { value: '', label: 'Все статусы' },
  { value: 'active', label: userStatusLabel('active') },
  { value: 'blocked', label: userStatusLabel('blocked') },
]

const MIN_REASON_LENGTH = 3

interface BlockModalProps {
  user: AdminUser
  onClose: () => void
}

function BlockUserModal({ user, onClose }: BlockModalProps) {
  const blockUser = useBlockUser()
  const [reason, setReason] = useState('')
  const trimmed = reason.trim()
  const canSubmit = trimmed.length >= MIN_REASON_LENGTH && !blockUser.isPending

  const handleSubmit = () => {
    if (!canSubmit) return
    blockUser.mutate(
      { userId: user.id, reason: trimmed },
      {
        onSuccess: () => {
          toast.success(`Пользователь ${user.email} заблокирован`)
          onClose()
        },
        onError: handleApiError,
      },
    )
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
      <div className="w-full max-w-lg rounded-lg bg-white shadow-xl">
        <div className="border-b bg-gray-50 p-4 rounded-t-lg">
          <h3 className="text-lg font-semibold text-gray-900">Блокировка пользователя</h3>
          <p className="mt-1 text-sm text-gray-500">{user.email} · {roleLabel(user.role)}</p>
        </div>
        <div className="p-4">
          <label htmlFor="block-reason" className="mb-1 block text-sm font-medium text-gray-700">
            Причина (обязательно, минимум {MIN_REASON_LENGTH} символа)
          </label>
          <textarea
            id="block-reason"
            rows={3}
            maxLength={2000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Например: подозрительная активность, запрос от службы безопасности"
            className="w-full rounded border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-200"
          />
        </div>
        <div className="flex justify-end gap-2 border-t bg-gray-50 p-4 rounded-b-lg">
          <Button variant="outline" onClick={onClose}>Отмена</Button>
          <Button variant="danger" disabled={!canSubmit} onClick={handleSubmit}>
            {blockUser.isPending ? 'Блокировка…' : 'Заблокировать'}
          </Button>
        </div>
      </div>
    </div>
  )
}

// Вкладка «Пользователи»: поиск по email с debounce, фильтры role/status,
// блокировка через модалку с причиной и разблокировка одним кликом. Себя блокировать нельзя.
export function UsersTab() {
  const { user: currentUser } = useAuth()
  const [search, setSearch] = useState('')
  const [role, setRole] = useState<'' | Schemas['Role']>('')
  const [status, setStatus] = useState<'' | Schemas['UserStatus']>('')
  const [blockingUser, setBlockingUser] = useState<AdminUser | null>(null)

  const debouncedSearch = useDebounce(search)

  const usersQuery = useAdminUsers({
    q: debouncedSearch.trim() || undefined,
    role: role || undefined,
    status: status || undefined,
  })
  const users = usersQuery.data?.items ?? []
  const unblockUser = useUnblockUser()

  const selectClass = 'rounded border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-200'

  const handleUnblock = (user: AdminUser) => {
    unblockUser.mutate(user.id, {
      onSuccess: () => toast.success(`Пользователь ${user.email} разблокирован`),
      onError: handleApiError,
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Поиск по email…"
          className="max-w-xs"
          data-testid="users-search"
        />
        <label className="flex items-center gap-2 text-sm text-gray-600">
          Роль
          <select
            value={role}
            onChange={(e) => setRole(e.target.value as '' | Schemas['Role'])}
            className={selectClass}
            data-testid="users-role-filter"
          >
            {ROLE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-600">
          Статус
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value as '' | Schemas['UserStatus'])}
            className={selectClass}
            data-testid="users-status-filter"
          >
            {STATUS_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </label>
        <span className="ml-auto text-sm text-gray-500">Всего: {usersQuery.data?.total ?? '—'}</span>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <table className="min-w-full divide-y divide-gray-200 text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Email</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Роль</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Статус</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500"> Регистрация</th>
              <th className="px-4 py-3 text-right font-medium text-gray-500">Действие</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white">
            {users.map((u) => {
              const isSelf = u.id === currentUser?.id
              return (
                <tr key={u.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3">
                    <span className="font-medium text-gray-900">{u.email}</span>
                    {isSelf && <span className="ml-2 text-xs text-gray-400">(вы)</span>}
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant={u.role === 'admin' ? 'info' : 'default'}>{roleLabel(u.role)}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant={u.status === 'blocked' ? 'danger' : 'success'}>
                      {userStatusLabel(u.status)}
                    </Badge>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap text-gray-500">
                    {new Date(u.created_at).toLocaleDateString(currentLocale())}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {u.status === 'active' ? (
                      <Button
                        size="sm"
                        variant="danger"
                        disabled={isSelf || unblockUser.isPending}
                        title={isSelf ? 'Нельзя заблокировать себя' : undefined}
                        onClick={() => setBlockingUser(u)}
                      >
                        Заблокировать
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={unblockUser.isPending}
                        onClick={() => handleUnblock(u)}
                      >
                        Разблокировать
                      </Button>
                    )}
                  </td>
                </tr>
              )
            })}
            {!usersQuery.isLoading && users.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-6 text-center text-gray-500">
                  Пользователей по фильтрам нет.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        <LoadMore query={usersQuery} />
      </div>

      {blockingUser && <BlockUserModal user={blockingUser} onClose={() => setBlockingUser(null)} />}
    </div>
  )
}
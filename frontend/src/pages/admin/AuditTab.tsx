import { useState } from 'react'
import { useAuditLog, type AuditLogEntry } from '@/hooks/useAdmin'
import { useDebounce } from '@/hooks/useDebounce'
import { auditActionLabel, ENTITY_TYPE_LABEL } from '@/lib/labels'
import type { components } from '@/lib/api-schema'
import { Badge } from '@/components/ui/Badge'
import { Input } from '@/components/ui/Input'
import { LoadMore } from '@/components/ui/LoadMore'
import { currentLocale } from '@/i18n'

type Schemas = components['schemas']
type EntityFilter = '' | Schemas['EntityType'] | 'system'

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

const ENTITY_OPTIONS: { value: EntityFilter; label: string }[] = [
  { value: '', label: 'Все сущности' },
  { value: 'system', label: 'Система (без автора)' },
  ...(Object.keys(ENTITY_TYPE_LABEL) as Schemas['EntityType'][]).map((t) => ({
    value: t as EntityFilter,
    label: ENTITY_TYPE_LABEL[t],
  })),
]

function formatDateTime(value: string | undefined) {
  if (!value) return '-'
  return new Date(value).toLocaleString(currentLocale(), { dateStyle: 'short', timeStyle: 'short' })
}

// Строка журнала без автора - системное действие (сиды, фоновые задачи).
function isSystemEntry(entry: AuditLogEntry) {
  return !entry.user_id
}

function AuditRow({ entry }: { entry: AuditLogEntry }) {
  return (
    <tr className="hover:bg-gray-50">
      <td className="px-4 py-3 whitespace-nowrap text-gray-500">{formatDateTime(entry.created_at)}</td>
      <td className="px-4 py-3">
        <Badge variant={isSystemEntry(entry) ? 'default' : 'info'}>{auditActionLabel(entry.action)}</Badge>
      </td>
      <td className="px-4 py-3 font-mono text-xs text-gray-500">
        {entry.entity_id ? entry.entity_id : '-'}
      </td>
      <td className="px-4 py-3 font-mono text-xs text-gray-500" >
        {entry.user_id ? entry.user_id : '-'}
      </td>
      <td className="px-4 py-3 text-xs text-gray-400" >
        {entry.ip ?? '-'}
      </td>
    </tr>
  )
}


// Вкладка «Аудит»: журнал действий с поиском по UUID автора и фильтрами по действию/сущности/периоду.
export function AuditTab() {
  const [userSearch, setUserSearch] = useState('')
  const [action, setAction] = useState('')
  const [entity, setEntity] = useState<EntityFilter>('')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')

  const debouncedUser = useDebounce(userSearch)
  const trimmedUser = debouncedUser.trim().toLowerCase()
  const trimmedAction = action.trim()

  const isUuid = UUID_RE.test(trimmedUser)
  const userFilter = trimmedUser && isUuid ? trimmedUser : undefined
  const entityFilter = trimmedUser && !isUuid ? trimmedUser : undefined

  const auditQuery = useAuditLog({
    user_id: userFilter,
    entity_id: entityFilter,
    action: trimmedAction || undefined,
    entity_type: entity === '' || entity === 'system' ? undefined : entity,
    created_from: dateFrom || undefined,
    created_to: dateTo || undefined,
  })

  const rawItems = auditQuery.data?.items ?? []
  const items = entity === 'system' ? rawItems.filter(isSystemEntry) : rawItems

  const selectClass =
    'rounded border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-200'

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <Input
          value={userSearch}
          onChange={(e) => setUserSearch(e.target.value)}
          placeholder="Поиск по UUID (автор или объект)…"
          className="max-w-xs"
          data-testid="audit-user-search"
        />
        <Input
          value={action}
          onChange={(e) => setAction(e.target.value)}
          placeholder="Действие, напр. offer.accept"
          className="max-w-[220px]"
          data-testid="audit-action-filter"
        />
        <label className="flex items-center gap-2 text-sm text-gray-600">
          Сущность
          <select
            value={entity}
            onChange={(e) => setEntity(e.target.value as EntityFilter)}
            className={selectClass}
            data-testid="audit-entity-filter"
          >
            {ENTITY_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-600">
          С даты
          <input
            type="date"
            value={dateFrom}
            onChange={(e) => setDateFrom(e.target.value)}
            className={selectClass}
            data-testid="audit-date-from"
          />
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-600">
          По дату
          <input
            type="date"
            value={dateTo}
            onChange={(e) => setDateTo(e.target.value)}
            className={selectClass}
            data-testid="audit-date-to"
          />
        </label>
        <span className="ml-auto text-sm text-gray-500">Всего: {auditQuery.data?.total ?? '-'}</span>
      </div>

      {trimmedUser && !isUuid && (
        <p className="text-xs text-gray-500">ID автора или объекта</p>
      )}

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <table className="min-w-full divide-y divide-gray-200 text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Дата</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Действие</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">ID объекта</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Пользователь</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">IP</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white">
            {items.map((entry) => (
              <AuditRow key={entry.id} entry={entry} />
            ))}
            {!auditQuery.isLoading && items.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-6 text-center text-gray-500">
                  Записей аудита по фильтрам нет.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        <LoadMore query={auditQuery} />
      </div>
    </div>
  )
}

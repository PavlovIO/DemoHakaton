import { useState } from 'react'
import { toast } from 'sonner'
import { useAmlFlags, useResolveAmlFlag, type AmlFlag } from '@/hooks/useAdmin'
import { handleApiError } from '@/lib/api-errors'
import { amlFlagStatusLabel, amlRuleLabel, amlSeverityLabel } from '@/lib/labels'
import type { components } from '@/lib/api-schema'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { LoadMore } from '@/components/ui/LoadMore'

type Schemas = components['schemas']
type AmlResolution = 'false_positive' | 'confirmed' // enum requestBody adminResolveAmlFlag

const STATUS_OPTIONS: { value: '' | Schemas['AmlFlagStatus']; label: string }[] = [
  { value: '', label: 'Все статусы' },
  { value: 'open', label: 'Открытые' },
  { value: 'false_positive', label: 'Ложное срабатывание' },
  { value: 'confirmed', label: 'Подтверждённые' },
]

const SEVERITY_OPTIONS: { value: '' | Schemas['AmlSeverity']; label: string }[] = [
  { value: '', label: 'Любая критичность' },
  { value: 'high', label: 'Высокий' },
  { value: 'medium', label: 'Средний' },
  { value: 'low', label: 'Низкий' },
]

// severity - цвет бейджа: high всегда бросается в глаза (блокирует выдачу KYC).
function severityVariant(severity: Schemas['AmlSeverity']) {
  if (severity === 'high') return 'danger' as const
  if (severity === 'medium') return 'warning' as const
  return 'info' as const
}

function statusVariant(status: Schemas['AmlFlagStatus']) {
  if (status === 'open') return 'warning' as const
  if (status === 'confirmed') return 'danger' as const
  return 'default' as const
}

// comment minLength=3 по контракту adminResolveAmlFlag — проверяем до отправки.
const MIN_COMMENT_LENGTH = 3

interface ResolveModalProps {
  flag: AmlFlag
  onClose: () => void
}

// Модалка снятия флага: резолвер + обязательный комментарий (3–2000 символов).
function ResolveFlagModal({ flag, onClose }: ResolveModalProps) {
  const resolveFlag = useResolveAmlFlag()
  const [resolution, setResolution] = useState<AmlResolution>('false_positive')
  const [comment, setComment] = useState('')

  const trimmed = comment.trim()
  const canSubmit = trimmed.length >= MIN_COMMENT_LENGTH && !resolveFlag.isPending

  const handleSubmit = () => {
    if (!canSubmit) return
    resolveFlag.mutate(
      { flagId: flag.id, resolution, comment: trimmed },
      {
        onSuccess: () => {
          toast.success(resolution === 'false_positive' ? 'Флаг закрыт как ложное срабатывание' : 'Флаг подтверждён')
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
          <h3 className="text-lg font-semibold text-gray-900">Снятие AML-флага #{flag.id}</h3>
          <p className="mt-1 text-sm text-gray-500">{flag.details}</p>
        </div>

        <div className="space-y-4 p-4">
          <fieldset>
            <legend className="mb-2 block text-sm font-medium text-gray-700">Решение</legend>
            <div className="flex gap-4">
              <label className="flex items-center gap-2 text-sm text-gray-900">
                <input
                  type="radio"
                  name="resolution"
                  checked={resolution === 'false_positive'}
                  onChange={() => setResolution('false_positive')}
                />
                Ложное срабатывание
              </label>
              <label className="flex items-center gap-2 text-sm text-gray-900">
                <input
                  type="radio"
                  name="resolution"
                  checked={resolution === 'confirmed'}
                  onChange={() => setResolution('confirmed')}
                />
                Подтвердить нарушение
              </label>
            </div>
          </fieldset>

          <div>
            <label htmlFor="aml-resolve-comment" className="mb-1 block text-sm font-medium text-gray-700">
              Комментарий (обязательно, минимум {MIN_COMMENT_LENGTH} символа)
            </label>
            <textarea
              id="aml-resolve-comment"
              rows={4}
              maxLength={2000}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder="Что проверили и почему приняли такое решение"
              className="w-full rounded border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-200"
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 border-t bg-gray-50 p-4 rounded-b-lg">
          <Button variant="outline" onClick={onClose}>Отмена</Button>
          <Button disabled={!canSubmit} onClick={handleSubmit}>
            {resolveFlag.isPending ? 'Сохранение…' : 'Снять флаг'}
          </Button>
        </div>
      </div>
    </div>
  )
}

// Вкладка «AML-флаги» : таблица со severity-бейджами, фильтры status/severity,
// снятие открытого флага через модалку с обязательным комментарием.
export function AmlFlagsTab() {
  const [status, setStatus] = useState<'' | Schemas['AmlFlagStatus']>('open')
  const [severity, setSeverity] = useState<'' | Schemas['AmlSeverity']>('')
  const [resolvingFlag, setResolvingFlag] = useState<AmlFlag | null>(null)

  const flagsQuery = useAmlFlags({
    status: status || undefined,
    severity: severity || undefined,
  })
  const flags = flagsQuery.data?.items ?? []

  const selectClass = 'rounded border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-200'

  return (
    <div className="space-y-4">
      {/* Фильтры */}
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm text-gray-600">
          Статус
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value as '' | Schemas['AmlFlagStatus'])}
            className={selectClass}
            data-testid="aml-status-filter"
          >
            {STATUS_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-600">
          Критичность
          <select
            value={severity}
            onChange={(e) => setSeverity(e.target.value as '' | Schemas['AmlSeverity'])}
            className={selectClass}
            data-testid="aml-severity-filter"
          >
            {SEVERITY_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </label>
        <span className="ml-auto text-sm text-gray-500">Всего: {flagsQuery.data?.total ?? '—'}</span>
      </div>

      <div className="bg-white shadow overflow-hidden sm:rounded-md">
        <table className="min-w-full divide-y divide-gray-200 text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Компания / заявка</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Правило</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Severity</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Статус</th>
              <th className="px-4 py-3 text-left font-medium text-gray-500">Дата</th>
              <th className="px-4 py-3 text-right font-medium text-gray-500">Действие</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white">
            {flags.map((flag) => (
              <tr key={flag.id} className="hover:bg-gray-50 align-top">
                <td className="px-4 py-3">
                  <div className="font-medium text-gray-900">{flag.organization.name}</div>
                  <div className="text-xs text-gray-500">{flag.organization.country_code}</div>
                  {flag.application_id && (
                    <div className="mt-0.5 font-mono text-xs text-gray-400" title={flag.application_id}>
                      заявка {flag.application_id.slice(0, 8)}…
                    </div>
                  )}
                </td>
                <td className="px-4 py-3">
                  <div className="text-gray-900">{amlRuleLabel(flag.rule)}</div>
                  <div className="mt-1 max-w-md text-xs text-gray-500">{flag.details}</div>
                  {flag.resolution_comment && (
                    <div className="mt-1 max-w-md text-xs text-gray-500 italic">
                      Решение: {flag.resolution_comment}
                    </div>
                  )}
                </td>
                <td className="px-4 py-3">
                  <Badge variant={severityVariant(flag.severity)}>{amlSeverityLabel(flag.severity)}</Badge>
                </td>
                <td className="px-4 py-3">
                  <Badge variant={statusVariant(flag.status)}>{amlFlagStatusLabel(flag.status)}</Badge>
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-gray-500">
                  {new Date(flag.created_at).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' })}
                </td>
                <td className="px-4 py-3 text-right">
                  {flag.status === 'open' ? (
                    <Button size="sm" variant="outline" onClick={() => setResolvingFlag(flag)}>
                      Снять флаг
                    </Button>
                  ) : (
                    <span className="text-xs text-gray-400">
                      {flag.resolved_at
                        ? `Закрыт ${new Date(flag.resolved_at).toLocaleDateString('ru-RU')}`
                        : 'Закрыт'}
                    </span>
                  )}
                </td>
              </tr>
            ))}
            {!flagsQuery.isLoading && flags.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-6 text-center text-gray-500">
                  Флагов по фильтрам нет.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        <LoadMore query={flagsQuery} />
      </div>

      {resolvingFlag && <ResolveFlagModal flag={resolvingFlag} onClose={() => setResolvingFlag(null)} />}
    </div>
  )
}

export function AmlFlagsForApplication({ applicationId }: { applicationId: string }) {
  const { data } = useAmlFlags({ application_id: applicationId })
  const flags = data?.items ?? []
  if (flags.length === 0) return null
  return (
    <ul className="mt-2 space-y-1">
      {flags.map((f) => (
        <li key={f.id} className="flex items-center gap-2 text-xs">
          <Badge variant={severityVariant(f.severity)}>{amlSeverityLabel(f.severity)}</Badge>
          <span className="text-gray-600">{amlRuleLabel(f.rule)}: {f.details}</span>
        </li>
      ))}
    </ul>
  )
}
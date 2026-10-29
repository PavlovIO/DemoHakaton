import type { ReactNode } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { type OrganizationDetails, useCompanyCard } from '@/hooks/useKyc'
import { kycStatusLabel, kycStatusVariant, roleLabel, userStatusLabel } from '@/lib/labels'
import { formatPercent } from '@/lib/money'
import { currentLocale } from '@/i18n'
import { ApiError } from '@/lib/api-client'
import { Badge } from '@/components/ui/Badge'
import { DocumentList } from '@/components/kyc/DocumentList'
import { AmlFlagList } from './AmlFlagsTab'
import { CompanyDecision } from './CompanyDecision'
import { companyAmlFlagsPath, companyCardOrigin } from './paths'

type RegistryCheck = NonNullable<OrganizationDetails['registry_check']>

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded-lg border border-gray-200 bg-white p-4">
      <h3 className="mb-3 text-lg font-medium text-gray-900">{title}</h3>
      {children}
    </section>
  )
}

function formatDateTime(value: string | undefined) {
  if (!value) return '—'
  return new Date(value).toLocaleString(currentLocale(), { dateStyle: 'short', timeStyle: 'short' })
}

// Сверка делается при отправке на проверку; до неё результата нет.
function RegistryCheckView({ check }: { check: RegistryCheck | null }) {
  if (!check) return <p className="text-sm text-gray-500">Сверка не проводилась.</p>
  if (!check.found) {
    return <p className="text-sm text-danger">В реестре не найдена (проверено {formatDateTime(check.checked_at)}).</p>
  }
  return (
    <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-sm">
      <dt className="text-gray-500">Название</dt>
      <dd>{check.name ?? '—'}</dd>
      <dt className="text-gray-500">Рег. номер</dt>
      <dd>{check.registration_number ?? '—'}</dd>
      <dt className="text-gray-500">Адрес</dt>
      <dd>{check.legal_address ?? '—'}</dd>
      <dt className="text-gray-500">Статус в реестре</dt>
      <dd>{check.registry_status ?? '—'}</dd>
      <dt className="text-gray-500">Источник</dt>
      <dd>{check.source} · {formatDateTime(check.checked_at)}</dd>
    </dl>
  )
}

// Карточка компании: всё, что администратор смотрит перед решением по проверке компании.
export function CompanyCard() {
  const { organizationId = '' } = useParams()
  const back = companyCardOrigin(useLocation().state)
  const { data: company, isLoading, error } = useCompanyCard(organizationId)

  if (isLoading) return <p className="text-gray-500">Загрузка…</p>
  if (!company) {
    if (error instanceof ApiError && error.status === 404) {
      return <p className="text-gray-500">Компания не найдена.</p>
    }
    return <p className="text-danger">Не удалось загрузить карточку компании.</p>
  }

  return (
    <div className="space-y-4">
      <Link to={back.to} className="text-sm text-gray-600 hover:text-gray-900">← {back.label}</Link>

      <div className="flex flex-wrap items-start justify-between gap-4 rounded-lg border border-gray-200 bg-white p-4">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">{company.name}</h2>
          <div className="mt-2 flex items-center gap-2">
            <Badge variant="default">{roleLabel(company.type)}</Badge>
            <Badge variant={kycStatusVariant(company.kyc_status)}>{kycStatusLabel(company.kyc_status)}</Badge>
          </div>
          {company.kyc_status === 'rejected' && company.kyc_reject_reason && (
            <p className="mt-2 text-sm text-danger">Причина отказа: {company.kyc_reject_reason}</p>
          )}
        </div>
        <CompanyDecision company={company} />
      </div>

      <Section title="Реквизиты">
        <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-gray-500">Страна</dt>
          <dd>{company.country_code}</dd>
          <dt className="text-gray-500">Рег. номер</dt>
          <dd>{company.registration_number}</dd>
          <dt className="text-gray-500">LEI</dt>
          <dd>{company.lei ?? '—'}</dd>
          <dt className="text-gray-500">Юр. адрес</dt>
          <dd>{company.legal_address}</dd>
        </dl>
      </Section>

      <Section title="Документы">
        <DocumentList documents={company.documents} />
      </Section>

      <Section title="Бенефициары">
        {company.beneficiaries.length === 0 ? (
          <p className="text-sm text-gray-500">Бенефициары не указаны.</p>
        ) : (
          <ul className="space-y-1 text-sm">
            {company.beneficiaries.map((b, i) => (
              <li key={i}>
                {b.full_name} · {b.citizenship_country_code} · {formatPercent(b.ownership_percent)}
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title="AML-флаги">
        {company.aml_flags.length === 0 ? (
          <p className="text-sm text-gray-500">Флагов нет.</p>
        ) : (
          <>
            <AmlFlagList flags={company.aml_flags} />
            <Link to={companyAmlFlagsPath(company.id)} className="mt-2 inline-block text-sm text-blue-600 hover:underline">
              Разобрать на вкладке AML-флагов →
            </Link>
          </>
        )}
      </Section>

      <Section title="Сверка с реестром">
        <RegistryCheckView check={company.registry_check} />
      </Section>

      <Section title="Пользователи">
        <ul className="space-y-1 text-sm">
          {company.users.map((u) => (
            <li key={u.id}>
              {u.email} · {roleLabel(u.role)} · {userStatusLabel(u.status)}
            </li>
          ))}
        </ul>
      </Section>
    </div>
  )
}

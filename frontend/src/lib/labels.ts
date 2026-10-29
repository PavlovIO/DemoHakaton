import type { components } from '@/lib/api-schema'
import { currentLanguage, i18n } from '@/i18n'

type RepaymentSchedule = components['schemas']['RepaymentSchedule']
type CountryCode = components['schemas']['CountryCode']
type TransferMethod = components['schemas']['TransferMethod']

type AmlRule = components['schemas']['AmlRule']
type AmlSeverity = components['schemas']['AmlSeverity']
type AmlFlagStatus = components['schemas']['AmlFlagStatus']
type Role = components['schemas']['Role']
type UserStatus = components['schemas']['UserStatus']
type ApplicationStatus = components['schemas']['ApplicationStatus']
type KycStatus = components['schemas']['KycStatus']
type OfferStatus = components['schemas']['OfferStatus']

// --- Справочники подписей для админ-вкладок (шаги 3C–3E). Ключи — строго из api-schema. ---

export const AML_RULE_LABEL: Record<AmlRule, string> = {
  sanctions_match: 'Совпадение с санкционным списком',
  high_risk_jurisdiction: 'Высокорисковая юрисдикция',
  amount_threshold: 'Превышение порога суммы',
  registry_mismatch: 'Несовпадение реестровых данных',
  other: 'Другое',
}

export const AML_SEVERITY_LABEL: Record<AmlSeverity, string> = {
  low: 'Низкий',
  medium: 'Средний',
  high: 'Высокий',
}

export const AML_FLAG_STATUS_LABEL: Record<AmlFlagStatus, string> = {
  open: 'Открыт',
  false_positive: 'Ложное срабатывание',
  confirmed: 'Подтверждён',
}

export const USER_STATUS_LABEL: Record<UserStatus, string> = {
  active: 'Активен',
  blocked: 'Заблокирован',
}

export function amlRuleLabel(rule: AmlRule | string): string {
  return AML_RULE_LABEL[rule as AmlRule] ?? rule
}

export function amlSeverityLabel(severity: AmlSeverity | string): string {
  return AML_SEVERITY_LABEL[severity as AmlSeverity] ?? severity
}

export function amlFlagStatusLabel(status: AmlFlagStatus | string): string {
  return AML_FLAG_STATUS_LABEL[status as AmlFlagStatus] ?? status
}

// Роли и статусы проверки компании видят клиенты — подписи из словаря i18n.
export function roleLabel(role: Role | string): string {
  const key = `labels.role.${role as Role}` as const
  return i18n.t(key, { defaultValue: role })
}

export function userStatusLabel(status: UserStatus | string): string {
  return USER_STATUS_LABEL[status as UserStatus] ?? status
}

export function applicationStatusLabel(status: ApplicationStatus | string): string {
  const key = `labels.applicationStatus.${status as ApplicationStatus}` as const
  return i18n.t(key, { defaultValue: status })
}

export function offerStatusLabel(status: OfferStatus | string): string {
  const key = `labels.offerStatus.${status as OfferStatus}` as const
  return i18n.t(key, { defaultValue: status })
}

export function kycStatusLabel(status: KycStatus | string): string {
  const key = `labels.kycStatus.${status as KycStatus}` as const
  return i18n.t(key, { defaultValue: status })
}

// Цвет бейджа статуса проверки: один у компании в её профиле и у админа в карточке компании.
const KYC_STATUS_VARIANT: Record<KycStatus, 'default' | 'warning' | 'success' | 'danger'> = {
  draft: 'default',
  under_review: 'warning',
  verified: 'success',
  rejected: 'danger',
}

export function kycStatusVariant(status: KycStatus) {
  return KYC_STATUS_VARIANT[status]
}

const FALLBACK_COUNTRY_NAME_RU: Partial<Record<CountryCode, string>> = {
  DE: 'Германия',
  CN: 'Китай',
  TR: 'Турция',
}

export const FALLBACK_TRANSFER_METHOD_NAME: Record<TransferMethod, string> = {
  swift: 'SWIFT-перевод',
  sepa: 'SEPA-перевод',
  spfs: 'СПФС Банка России',
  cips: 'CIPS (Китай)',
  other: 'Другое (указать в комментарии)',
}

// Порядок вариантов в форме оффера; подписи — labels.repaymentSchedule.* в словаре.
export const REPAYMENT_SCHEDULES: RepaymentSchedule[] = [
  'bullet',
  'annuity',
  'interest_monthly_principal_bullet',
  'custom',
]

export function repaymentScheduleLabel(schedule: RepaymentSchedule | string): string {
  const key = `labels.repaymentSchedule.${schedule as RepaymentSchedule}` as const
  return i18n.t(key, { defaultValue: schedule })
}

// Справочник стран отдаёт оба названия — берём на языке интерфейса.
export function countryName(country: { name_ru: string; name_en: string }): string {
  return currentLanguage() === 'en' ? country.name_en : country.name_ru
}

// В справочнике валют название только русское; английское — из Intl по ISO-коду.
export function currencyName(currency: { code: string; name: string }): string {
  if (currentLanguage() !== 'en') return currency.name
  try {
    return new Intl.DisplayNames(['en'], { type: 'currency' }).of(currency.code) ?? currency.name
  } catch {
    return currency.name
  }
}

export function countryLabel(code: CountryCode, countries?: { code: string; name_ru: string }[]): string {
  const name = countries?.find((c) => c.code === code)?.name_ru ?? FALLBACK_COUNTRY_NAME_RU[code]
  return name ? `${name} (${code})` : code
}

export function transferMethodLabel(code: TransferMethod | string, methods?: { code: string; name: string }[]): string {
  return (
    methods?.find((m) => m.code === code)?.name ??
    FALLBACK_TRANSFER_METHOD_NAME[code as TransferMethod] ??
    code
  )
}
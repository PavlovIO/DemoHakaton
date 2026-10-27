import type { components } from '@/lib/api-schema'

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

export const ROLE_LABEL: Record<Role, string> = {
  admin: 'Администратор',
  borrower: 'Заёмщик',
  lender: 'Кредитор',
}

export const USER_STATUS_LABEL: Record<UserStatus, string> = {
  active: 'Активен',
  blocked: 'Заблокирован',
}

export const APPLICATION_STATUS_LABEL: Record<ApplicationStatus, string> = {
  draft: 'Черновик',
  moderation: 'На модерации',
  rejected: 'Отклонена',
  published: 'Опубликована',
  agreed: 'Согласована',
  cancelled: 'Отменена',
}

export const KYC_STATUS_LABEL: Record<KycStatus, string> = {
  draft: 'Черновик',
  under_review: 'На проверке',
  verified: 'Верифицирована',
  rejected: 'Отклонена',
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

export function roleLabel(role: Role | string): string {
  return ROLE_LABEL[role as Role] ?? role
}

export function userStatusLabel(status: UserStatus | string): string {
  return USER_STATUS_LABEL[status as UserStatus] ?? status
}

export function applicationStatusLabel(status: ApplicationStatus | string): string {
  return APPLICATION_STATUS_LABEL[status as ApplicationStatus] ?? status
}

export function kycStatusLabel(status: KycStatus | string): string {
  return KYC_STATUS_LABEL[status as KycStatus] ?? status
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

export const REPAYMENT_SCHEDULE_LABEL: Record<RepaymentSchedule, string> = {
  bullet: 'В конце срока',
  annuity: 'Аннуитетный',
  interest_monthly_principal_bullet: 'Проценты ежемесячно, тело в конце',
  custom: 'Индивидуальный',
}

export function repaymentScheduleLabel(schedule: RepaymentSchedule | string): string {
  return REPAYMENT_SCHEDULE_LABEL[schedule as RepaymentSchedule] ?? schedule
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
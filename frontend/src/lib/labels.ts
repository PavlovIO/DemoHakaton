import type { components } from '@/lib/api-schema'

type RepaymentSchedule = components['schemas']['RepaymentSchedule']
type CountryCode = components['schemas']['CountryCode']
type TransferMethod = components['schemas']['TransferMethod']

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
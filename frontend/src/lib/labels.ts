import type { components } from '@/lib/api-schema'

type RepaymentSchedule = components['schemas']['RepaymentSchedule']

// Человекочитаемые метки для enum-значений из API.
// Ключевые значения (id в URL, значения фильтров) остаются как в контракте — меняем только отображение.
export const REPAYMENT_SCHEDULE_LABEL: Record<RepaymentSchedule, string> = {
  bullet: 'В конце срока',
  annuity: 'Аннуитетный',
  interest_monthly_principal_bullet: 'Проценты ежемесячно, тело в конце',
  custom: 'Индивидуальный',
}

export function repaymentScheduleLabel(schedule: RepaymentSchedule | string): string {
  return REPAYMENT_SCHEDULE_LABEL[schedule as RepaymentSchedule] ?? schedule
}

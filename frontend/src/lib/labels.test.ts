import { describe, expect, it } from 'vitest'
import {
  amlFlagStatusLabel,
  amlRuleLabel,
  amlSeverityLabel,
  applicationStatusLabel,
  countryLabel,
  kycStatusLabel,
  repaymentScheduleLabel,
  roleLabel,
  transferMethodLabel,
  userStatusLabel,
} from './labels'

describe('метки справочников', () => {
  it('countryLabel: известный код — «Название (CODE)», неизвестный — сам код', () => {
    expect(countryLabel('DE')).toBe('Германия (DE)')
    expect(countryLabel('XX')).toBe('XX')
  })

  it('transferMethodLabel: известные коды из сида, fallback — код', () => {
    expect(transferMethodLabel('swift')).toBe('SWIFT-перевод')
    expect(transferMethodLabel('sepa')).toBe('SEPA-перевод')
    expect(transferMethodLabel('unknown_code')).toBe('unknown_code')
  })

  it('repaymentScheduleLabel: enum → человекочитаемая метка, неизвестное — как есть', () => {
    expect(repaymentScheduleLabel('interest_monthly_principal_bullet')).toBe(
      'Проценты ежемесячно, тело в конце',
    )
    expect(repaymentScheduleLabel('bullet')).toBe('В конце срока')
    expect(repaymentScheduleLabel('something_new')).toBe('something_new')
  })
})
describe('подписи админ-вкладок (шаги 3C–3E)', () => {
  it('AML: правила, severity и статусы флага; неизвестное значение — как есть', () => {
    expect(amlRuleLabel('sanctions_match')).toBe('Совпадение с санкционным списком')
    expect(amlRuleLabel('high_risk_jurisdiction')).toBe('Высокорисковая юрисдикция')
    expect(amlRuleLabel('amount_threshold')).toBe('Превышение порога суммы')
    expect(amlRuleLabel('registry_mismatch')).toBe('Несовпадение реестровых данных')
    expect(amlRuleLabel('other')).toBe('Другое')
    expect(amlRuleLabel('future_rule')).toBe('future_rule')

    expect(amlSeverityLabel('high')).toBe('Высокий')
    expect(amlSeverityLabel('medium')).toBe('Средний')
    expect(amlSeverityLabel('low')).toBe('Низкий')

    expect(amlFlagStatusLabel('open')).toBe('Открыт')
    expect(amlFlagStatusLabel('false_positive')).toBe('Ложное срабатывание')
    expect(amlFlagStatusLabel('confirmed')).toBe('Подтверждён')
  })

  it('пользователи: роли и статусы', () => {
    expect(roleLabel('admin')).toBe('Администратор')
    expect(roleLabel('borrower')).toBe('Заёмщик')
    expect(roleLabel('lender')).toBe('Кредитор')
    expect(userStatusLabel('active')).toBe('Активен')
    expect(userStatusLabel('blocked')).toBe('Заблокирован')
  })

  it('аналитика: каждый статус заявок и KYC подписан, а не прокинут кодом', () => {
    for (const status of ['draft', 'moderation', 'rejected', 'published', 'agreed', 'cancelled'] as const) {
      expect(applicationStatusLabel(status)).not.toBe(status)
    }
    for (const status of ['draft', 'under_review', 'verified', 'rejected'] as const) {
      expect(kycStatusLabel(status)).not.toBe(status)
    }
  })
})

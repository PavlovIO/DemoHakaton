import { describe, expect, it } from 'vitest'
import { countryLabel, repaymentScheduleLabel, transferMethodLabel } from './labels'

describe('метки справочников', () => {
  it('countryLabel: известный код — «Название (CODE)», неизвестный — сам код', () => {
    expect(countryLabel('DE')).toBe('Германия (DE)')
    // Fallback без сети: список из /dictionaries/countries может не загрузиться.
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

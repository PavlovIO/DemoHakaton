import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { Select, type SelectOption } from './Select'

const options: SelectOption[] = [
  { value: '', label: 'Все страны' },
  { value: 'DE', label: 'Germany (DE)' },
  { value: 'IR', label: 'Iran (IR)', hint: 'FATF: повышенный риск' },
]

describe('Select (кастомный выбор в фильтрах)', () => {
  it('показывает placeholder, когда значение не выбрано', () => {
    render(<Select value="" onChange={() => {}} options={options.slice(1)} placeholder="Все страны" />)
    expect(screen.getByRole('button', { name: /Все страны/ })).toBeTruthy()
  })

  it('показывает выбранную опцию на кнопке', () => {
    render(<Select value="DE" onChange={() => {}} options={options} />)
    expect(screen.getByRole('button', { name: /Germany \(DE\)/ })).toBeTruthy()
  })

  it('открывает список по клику и выбирает опцию', () => {
    const onChange = vi.fn()
    render(<Select value="" onChange={onChange} options={options} />)
    fireEvent.click(screen.getByRole('button'))
    const listbox = screen.getByRole('listbox')
    expect(within(listbox).getAllByRole('option')).toHaveLength(3)
    fireEvent.click(within(listbox).getByRole('option', { name: /Iran/ }))
    expect(onChange).toHaveBeenCalledWith('IR')
    // После выбора панель закрывается.
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('закрывает по Escape и по клику вне списка', () => {
    render(<Select value="" onChange={() => {}} options={options} />)
    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByRole('listbox')).toBeTruthy()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('listbox')).toBeNull()

    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByRole('listbox')).toBeTruthy()
    fireEvent.pointerDown(document.body)
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('подсвечивает активную опцию и выводит hint вторострочным текстом', () => {
    render(<Select value="IR" onChange={() => {}} options={options} />)
    fireEvent.click(screen.getByRole('button'))
    const active = screen.getByRole('option', { name: /Iran/ })
    expect(active.getAttribute('aria-selected')).toBe('true')
    expect(active.textContent).toContain('FATF: повышенный риск')
  })

  it('скрытое нативное поле хранит текущее значение', () => {
    const { container } = render(<Select name="filter_country" value="DE" onChange={() => {}} options={options} />)
    const hidden = container.querySelector<HTMLInputElement>('input[type="hidden"][name="filter_country"]')
    expect(hidden?.value).toBe('DE')
  })

  it('disabled-выбор не открывает список', () => {
    render(<Select value="" onChange={() => {}} options={options} disabled />)
    fireEvent.click(screen.getByRole('button'))
    expect(screen.queryByRole('listbox')).toBeNull()
  })
})

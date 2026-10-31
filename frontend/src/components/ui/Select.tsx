import { type CSSProperties, useEffect, useRef, useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface SelectOption {
  value: string
  label: string
  hint?: string
}

interface SelectProps {
  name?: string
  value: string
  onChange: (value: string) => void
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
  className?: string
  id?: string
  'aria-labelledby'?: string
}

/**
 * Кастомный выпадающий список вместо нативного <select>.
 * Мобильные особенности:
 * - панель позиционируется fixed по координатам кнопки и при необходимости
 *   открывается вверх, если снизу не хватает места на экране;
 * - опции — это <button>, а не мелкий чекбокс: попадание пальцем по всей ширине;
 * - закрытие по клику вне, Escape и потере фокуса (touch-friendly blur).
 */
export function Select({
  name,
  value,
  onChange,
  options,
  placeholder,
  disabled,
  className,
  id,
  'aria-labelledby': ariaLabelledBy,
}: SelectProps) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const [panelStyle, setPanelStyle] = useState<CSSProperties>({})

  const selected = options.find((o) => o.value === value)

  const updatePosition = () => {
    const btn = buttonRef.current
    if (!btn) return
    const rect = btn.getBoundingClientRect()
    const estimatedHeight = Math.min(options.length * 48 + 16, 280)
    const spaceBelow = window.innerHeight - rect.bottom
    const spaceAbove = rect.top
    if (spaceBelow < estimatedHeight + 8 && spaceAbove > spaceBelow) {
      setPanelStyle({
        position: 'fixed',
        left: rect.left,
        width: rect.width,
        bottom: window.innerHeight - rect.top + 4,
        maxHeight: `min(${spaceAbove - 8}px, 70dvh)`,
      })
    } else {
      setPanelStyle({
        position: 'fixed',
        left: rect.left,
        width: rect.width,
        top: rect.bottom + 4,
        maxHeight: `min(${spaceBelow - 8}px, 70dvh)`,
      })
    }
  }

  useEffect(() => {
    if (!open) return
    updatePosition()
    const onPointerDown = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
        buttonRef.current?.focus()
      }
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    const active = listRef.current?.querySelector('[data-active="true"]')
    if (active && typeof active.scrollIntoView === 'function') {
      active.scrollIntoView({ block: 'nearest' })
    }
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  useEffect(() => {
    if (!open) return
    let raf = 0
    const reposition = () => {
      cancelAnimationFrame(raf)
      raf = requestAnimationFrame(updatePosition)
    }
    window.addEventListener('scroll', reposition, { capture: true, passive: true })
    window.addEventListener('resize', reposition, { passive: true })
    visualViewport?.addEventListener('scroll', reposition, { passive: true })
    visualViewport?.addEventListener('resize', reposition, { passive: true })
    return () => {
      cancelAnimationFrame(raf)
      window.removeEventListener('scroll', reposition, { capture: true })
      window.removeEventListener('resize', reposition)
      visualViewport?.removeEventListener('scroll', reposition)
      visualViewport?.removeEventListener('resize', reposition)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const pick = (optionValue: string) => {
    onChange(optionValue)
    setOpen(false)
    buttonRef.current?.focus()
  }

  return (
    <div ref={rootRef} className={cn('relative', className)}>
      <input type="hidden" name={name} value={value} />
      <button
        ref={buttonRef}
        type="button"
        id={id}
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-labelledby={ariaLabelledBy}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'flex min-h-11 w-full items-center justify-between gap-2 rounded-md border border-gray-300 bg-white px-3 py-2 text-left text-sm shadow-sm',
          'focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200',
          'disabled:bg-gray-100 disabled:text-gray-500',
        )}
      >
        <span className={cn('truncate', selected ? 'text-gray-900' : 'text-gray-500')}>
          {selected ? selected.label : (placeholder ?? '')}
        </span>
        <ChevronDown className="h-4 w-4 shrink-0 text-gray-400 transition-transform" aria-hidden />
      </button>

      {open && (
        <div
          ref={listRef}
          role="listbox"
          aria-selected={undefined}
          style={panelStyle}
          className="z-50 overflow-y-auto overscroll-contain rounded-md border border-gray-200 bg-white py-1 shadow-lg"
        >
          {options.map((option) => {
            const isActive = option.value === value
            return (
              <button
                key={option.value || '__empty__'}
                type="button"
                role="option"
                aria-selected={isActive}
                data-active={isActive || undefined}
                onClick={() => pick(option.value)}
                className={cn(
                  'flex min-h-11 w-full items-center gap-2 px-3 py-2 text-left text-sm',
                  isActive ? 'bg-blue-50 font-medium text-blue-700' : 'text-gray-900 hover:bg-gray-50 active:bg-gray-100',
                )}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{option.label}</span>
                  {option.hint && <span className="block truncate text-xs text-gray-500">{option.hint}</span>}
                </span>
                {isActive && <Check className="h-4 w-4 shrink-0" aria-hidden />}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
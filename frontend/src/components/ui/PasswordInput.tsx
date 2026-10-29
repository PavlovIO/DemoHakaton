import { forwardRef, useState, type InputHTMLAttributes } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Input } from './Input'

interface PasswordInputProps extends InputHTMLAttributes<HTMLInputElement> {
  className?: string
}

// Поле пароля с переключателем видимости («глазик»). Тип поля переключаем локальным
// состоянием: значение не теряется, автокомплит браузера работает по autoComplete.
export const PasswordInput = forwardRef<HTMLInputElement, PasswordInputProps>(
  ({ className, ...props }, ref) => {
    const [visible, setVisible] = useState(false)
    const { t } = useTranslation()

    return (
      <div className={cn('relative', className)}>
        <Input
          ref={ref}
          {...props}
          type={visible ? 'text' : 'password'}
          // Освобождаем место под кнопку справа, чтобы текст не залезал под иконку.
          className="pr-10"
        />
        <button
          type="button"
          onClick={() => setVisible((v) => !v)}
          aria-label={visible ? t('auth.passwordField.hide') : t('auth.passwordField.show')}
          aria-pressed={visible}
          className="absolute inset-y-0 right-0 flex w-10 items-center justify-center text-gray-400 hover:text-gray-600 focus:outline-none"
        >
          {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      </div>
    )
  },
)
PasswordInput.displayName = 'PasswordInput'

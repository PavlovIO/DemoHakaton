import { useEffect} from 'react'
import { useNavigate, useLocation, Link } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useAuth } from '@/hooks/useAuth'
import { validationMessage } from '@/i18n'
import { Button } from '@/components/ui/Button'
import { FieldError } from '@/components/ui/FieldError'
import { Input } from '@/components/ui/Input'
import { PasswordInput } from '@/components/ui/PasswordInput'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'

const loginSchema = z.object({
  email: z.email(validationMessage('validation.email')),
  password: z.string().min(6, validationMessage('validation.passwordMin6'))
})

type LoginForm = z.infer<typeof loginSchema>

export function Login() {
  const { t } = useTranslation()
  const { login, isAuthenticated, isLoading: authLoading } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string})?.from ?? '/dashboard'

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
  })

  useEffect(() => {
    if (isAuthenticated) navigate(from, {replace: true})
  }, [isAuthenticated, navigate, from])

  const onSubmit = async (data: LoginForm) => {
    await login(data.email, data.password)
    navigate(from, { replace: true })
  }

  if (authLoading) {
    return <div className="flex min-h-screen items-center justify-center">{t('common.loading')}</div>
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center p-4">
      <LanguageSwitcher className="absolute right-4 top-4" />
      <form
        onSubmit={handleSubmit(onSubmit)}
        className="w-full max-w-sm space-y-4 rounded-lg bg-white p-6 shadow"
      >
        <h1 className="text-xl font-semibold">{t('auth.login.title')}</h1>

        <div>
          <label className="text-sm">{t('auth.email')}</label>
          <Input {...register('email')} type="email" autoComplete="email" />
          <FieldError message={errors.email?.message} />
        </div>

        <div>
          <label className="text-sm">{t('auth.password')}</label>
          <PasswordInput {...register('password')} autoComplete="current-password" />
          <FieldError message={errors.password?.message} />
        </div>

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? t('auth.login.submitting') : t('auth.login.submit')}
        </Button>

        <p className="text-center text-sm text-gray-600">
          {t('auth.login.noAccount')}{' '}
          <Link to="/register" className="text-blue-600 hover:underline">
            {t('auth.login.toRegister')}
          </Link>
        </p>
      </form>
    </div>
  )
}

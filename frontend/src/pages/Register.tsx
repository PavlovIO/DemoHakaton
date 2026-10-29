import { useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth, type RegisterPayload } from '@/hooks/useAuth'
import { validationMessage } from '@/i18n'
import { Button } from '@/components/ui/Button'
import { FieldError } from '@/components/ui/FieldError'
import { Input } from '@/components/ui/Input'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'

const baseSchema = z.object({
  email: z.string().email(validationMessage('validation.email')),
  password: z.string().min(8, validationMessage('validation.passwordMin8')),
  role: z.enum(['borrower', 'lender']),
  organization: z.object({
    name: z.string().min(2, validationMessage('validation.companyName')),
    country_code: z
      .string()
      .length(2, validationMessage('validation.countryCode'))
      .toUpperCase(),
    legal_address: z.string().min(5, validationMessage('validation.legalAddress')),
    registration_number: z.string().min(3, validationMessage('validation.registrationNumber')),
    lei: z.string().optional(),
  }),
})

const schema = baseSchema.superRefine((data, ctx) => {
  if (data.role === 'borrower' && data.organization.country_code !== 'RU') {
    ctx.addIssue({
      code: 'custom', // вместо z.ZodIssueCode.custom
      path: ['organization', 'country_code'],
      message: validationMessage('validation.borrowerFromRu'),
    })
  }
  if (data.role === 'lender' && data.organization.country_code === 'RU') {
    ctx.addIssue({
      code: 'custom',
      path: ['organization', 'country_code'],
      message: validationMessage('validation.lenderNotFromRu'),
    })
  }
})

type RegisterForm = z.infer<typeof schema>

export function Register() {
  const { t } = useTranslation()
  const { register: registerApi } = useAuth()
  const navigate = useNavigate()

  const {
    register,
    handleSubmit,
    control,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<RegisterForm>({
    resolver: zodResolver(schema),
    defaultValues: {
      role: 'borrower',
      organization: {
        country_code: 'RU',
      },
    },
  })

  const role = useWatch({ control, name: 'role' })

  const onRoleChange = (newRole: 'borrower' | 'lender') => {
    setValue('role', newRole)
    setValue('organization.country_code', newRole === 'borrower' ? 'RU' : '')
  }

  const onSubmit = async (data: RegisterForm) => {
    const payload: RegisterPayload = {
      email: data.email,
      password: data.password,
      role: data.role,
      organization: {
        name: data.organization.name,
        country_code: data.organization.country_code.toUpperCase(),
        legal_address: data.organization.legal_address,
        registration_number: data.organization.registration_number,
        lei: data.organization.lei,
      },
    }
    await registerApi(payload)
    navigate('/dashboard', { replace: true })
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center p-4">
      <LanguageSwitcher className="absolute right-4 top-4" />
      <form
        onSubmit={handleSubmit(onSubmit)}
        className="w-full max-w-md space-y-4 rounded-lg bg-white p-6 shadow"
      >
        <h1 className="text-xl font-semibold">{t('auth.register.title')}</h1>

        {/* Выбор роли */}
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => onRoleChange('borrower')}
            className={`flex-1 rounded border px-3 py-2 text-sm ${
              role === 'borrower'
                ? 'border-blue-600 bg-blue-50 text-blue-700'
                : 'border-gray-300 text-gray-600'
            }`}
          >
            {t('auth.register.borrower')}
          </button>
          <button
            type="button"
            onClick={() => onRoleChange('lender')}
            className={`flex-1 rounded border px-3 py-2 text-sm ${
              role === 'lender'
                ? 'border-blue-600 bg-blue-50 text-blue-700'
                : 'border-gray-300 text-gray-600'
            }`}
          >
            {t('auth.register.lender')}
          </button>
        </div>

        {/* Данные компании */}
        <fieldset className="space-y-3 rounded border border-gray-200 p-3">
          <legend className="px-2 text-sm font-medium text-gray-700">
            {t('auth.register.company')}
          </legend>

          <div>
            <label className="text-sm">{t('auth.register.companyName')}</label>
            <Input {...register('organization.name')} />
            <FieldError message={errors.organization?.name?.message} />
          </div>

          <div>
            <label className="text-sm">
              {t('auth.register.countryCode')}{' '}
              <span className="text-gray-500">
                {role === 'borrower' ? t('auth.register.countryOnlyRu') : t('auth.register.countryNotRu')}
              </span>
            </label>
            <Input
              {...register('organization.country_code')}
              maxLength={2}
              disabled={role === 'borrower'}
              className="uppercase"
            />
            <FieldError message={errors.organization?.country_code?.message} />
          </div>

          <div>
            <label className="text-sm">{t('auth.register.legalAddress')}</label>
            <Input {...register('organization.legal_address')} />
            <FieldError message={errors.organization?.legal_address?.message} />
          </div>

          <div>
            <label className="text-sm">{t('auth.register.registrationNumber')}</label>
            <Input {...register('organization.registration_number')} />
            <FieldError message={errors.organization?.registration_number?.message} />
          </div>

          <div>
            <label className="text-sm">
              {t('auth.register.lei')} <span className="text-gray-500">{t('auth.register.optional')}</span>
            </label>
            <Input {...register('organization.lei')} />
          </div>
        </fieldset>

        {/* Данные пользователя */}
        <fieldset className="space-y-3 rounded border border-gray-200 p-3">
          <legend className="px-2 text-sm font-medium text-gray-700">
            {t('auth.register.account')}
          </legend>

          <div>
            <label className="text-sm">{t('auth.email')}</label>
            <Input {...register('email')} type="email" autoComplete="email" />
            <FieldError message={errors.email?.message} />
          </div>

          <div>
            <label className="text-sm">{t('auth.password')}</label>
            <Input {...register('password')} type="password" autoComplete="new-password" />
            <FieldError message={errors.password?.message} />
          </div>
        </fieldset>

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? t('auth.register.submitting') : t('auth.register.submit')}
        </Button>

        <p className="text-center text-sm text-gray-600">
          {t('auth.register.haveAccount')}{' '}
          <Link to="/login" className="text-blue-600 hover:underline">
            {t('auth.register.toLogin')}
          </Link>
        </p>
      </form>
    </div>
  )
}
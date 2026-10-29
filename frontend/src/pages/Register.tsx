import { useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth, type RegisterPayload } from '@/hooks/useAuth'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'

const baseSchema = z.object({
  email: z.string().email('Некорректный email'),
  password: z.string().min(8, 'Минимум 8 символов'),
  role: z.enum(['borrower', 'lender']),
  organization: z.object({
    name: z.string().min(2, 'Укажите название компании'),
    country_code: z
      .string()
      .length(2, 'Код страны — 2 буквы (ISO 3166-1 alpha-2)')
      .toUpperCase(),
    legal_address: z.string().min(5, 'Укажите юридический адрес'),
    registration_number: z.string().min(3, 'Укажите регистрационный номер'),
    lei: z.string().optional(),
  }),
})

const schema = baseSchema.superRefine((data, ctx) => {
  if (data.role === 'borrower' && data.organization.country_code !== 'RU') {
    ctx.addIssue({
      code: 'custom', // вместо z.ZodIssueCode.custom
      path: ['organization', 'country_code'],
      message: 'Заёмщик должен быть из России (RU)',
    })
  }
  if (data.role === 'lender' && data.organization.country_code === 'RU') {
    ctx.addIssue({
      code: 'custom',
      path: ['organization', 'country_code'],
      message: 'Зарубежный кредитор не может быть из России',
    })
  }
})

type RegisterForm = z.infer<typeof schema>

export function Register() {
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
    <div className="flex min-h-screen items-center justify-center p-4">
      <form
        onSubmit={handleSubmit(onSubmit)}
        className="w-full max-w-md space-y-4 rounded-lg bg-white p-6 shadow"
      >
        <h1 className="text-xl font-semibold">Регистрация</h1>

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
            Заёмщик (РФ)
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
            Кредитор (зарубежный)
          </button>
        </div>

        {/* Данные компании */}
        <fieldset className="space-y-3 rounded border border-gray-200 p-3">
          <legend className="px-2 text-sm font-medium text-gray-700">
            Данные компании
          </legend>

          <div>
            <label className="text-sm">Название компании</label>
            <Input {...register('organization.name')} />
            {errors.organization?.name && (
              <p className="mt-1 text-xs text-red-600">{errors.organization.name.message}</p>
            )}
          </div>

          <div>
            <label className="text-sm">
              Код страны (ISO 3166-1 alpha-2){' '}
              <span className="text-gray-500">
                {role === 'borrower' ? '— только RU' : '— не RU'}
              </span>
            </label>
            <Input
              {...register('organization.country_code')}
              maxLength={2}
              disabled={role === 'borrower'}
              className="uppercase"
            />
            {errors.organization?.country_code && (
              <p className="mt-1 text-xs text-red-600">
                {errors.organization.country_code.message}
              </p>
            )}
          </div>

          <div>
            <label className="text-sm">Юридический адрес</label>
            <Input {...register('organization.legal_address')} />
            {errors.organization?.legal_address && (
              <p className="mt-1 text-xs text-red-600">
                {errors.organization.legal_address.message}
              </p>
            )}
          </div>

          <div>
            <label className="text-sm">Регистрационный номер</label>
            <Input {...register('organization.registration_number')} />
            {errors.organization?.registration_number && (
              <p className="mt-1 text-xs text-red-600">
                {errors.organization.registration_number.message}
              </p>
            )}
          </div>

          <div>
            <label className="text-sm">
              LEI <span className="text-gray-500">(опционально)</span>
            </label>
            <Input {...register('organization.lei')} />
          </div>
        </fieldset>

        {/* Данные пользователя */}
        <fieldset className="space-y-3 rounded border border-gray-200 p-3">
          <legend className="px-2 text-sm font-medium text-gray-700">
            Учётная запись
          </legend>

          <div>
            <label className="text-sm">Email</label>
            <Input {...register('email')} type="email" autoComplete="email" />
            {errors.email && (
              <p className="mt-1 text-xs text-red-600">{errors.email.message}</p>
            )}
          </div>

          <div>
            <label className="text-sm">Пароль</label>
            <Input {...register('password')} type="password" autoComplete="new-password" />
            {errors.password && (
              <p className="mt-1 text-xs text-red-600">{errors.password.message}</p>
            )}
          </div>
        </fieldset>

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? 'Регистрация…' : 'Создать аккаунт'}
        </Button>

        <p className="text-center text-sm text-gray-600">
          Уже есть аккаунт?{' '}
          <Link to="/login" className="text-blue-600 hover:underline">
            Войти
          </Link>
        </p>
      </form>
    </div>
  )
}
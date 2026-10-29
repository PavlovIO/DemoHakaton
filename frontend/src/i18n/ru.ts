import type { components } from '@/lib/api-schema'

type Role = components['schemas']['Role']
type KycStatus = components['schemas']['KycStatus']
type KycDocumentType = components['schemas']['KycDocumentType']

// Русский словарь — эталон: en.ts обязан повторить его ключи (тип Dictionary).
// Сообщения валидации — ключи validation.*, их переводит FieldError при показе.
export const ru = {
  common: {
    loading: 'Загрузка…',
    delete: 'Удалить',
    add: 'Добавить',
  },
  language: {
    label: 'Язык интерфейса',
  },
  labels: {
    role: {
      admin: 'Администратор',
      borrower: 'Заёмщик',
      lender: 'Кредитор',
    } satisfies Record<Role, string>,
    kycStatus: {
      draft: 'Черновик',
      under_review: 'На проверке',
      verified: 'Подтверждена',
      rejected: 'Отклонена',
    } satisfies Record<KycStatus, string>,
  },
  auth: {
    email: 'Email',
    password: 'Пароль',
    login: {
      title: 'Вход в платформу ликвидности',
      submit: 'Войти',
      submitting: 'Вход…',
      noAccount: 'Нет аккаунта?',
      toRegister: 'Зарегистрироваться',
    },
    register: {
      title: 'Регистрация',
      borrower: 'Заёмщик (РФ)',
      lender: 'Кредитор (зарубежный)',
      company: 'Данные компании',
      companyName: 'Название компании',
      countryCode: 'Код страны (ISO 3166-1 alpha-2)',
      countryOnlyRu: '— только RU',
      countryNotRu: '— не RU',
      legalAddress: 'Юридический адрес',
      registrationNumber: 'Регистрационный номер',
      lei: 'LEI',
      optional: '(опционально)',
      account: 'Учётная запись',
      submit: 'Создать аккаунт',
      submitting: 'Регистрация…',
      haveAccount: 'Уже есть аккаунт?',
      toLogin: 'Войти',
    },
  },
  profileMenu: {
    companyProfile: 'Профиль компании',
    startVerification: 'Пройти проверку компании',
    logout: 'Выйти',
  },
  kyc: {
    loadingProfile: 'Загрузка профиля…',
    noCompany: {
      title: 'Профиль компании',
      text: 'У вашей учётной записи пока нет компании. Заполните профиль, чтобы получить доступ к заявкам и офферам.',
      action: 'Заполнить профиль',
    },
    title: 'Проверка компании',
    country: 'Страна: {{value}}',
    registrationNumber: 'Рег. номер: {{value}}',
    legalAddress: 'Юр. адрес: {{value}}',
    status: 'Статус:',
    rejected: {
      title: 'Проверка не пройдена',
      reason: 'Причина: {{reason}}',
      hint: 'Исправьте документы и отправьте компанию на проверку повторно.',
    },
    underReview: {
      title: 'Документы на проверке',
      text: 'Администратор проверит ваши документы в ближайшее время. Обычно это занимает до 24 часов.',
    },
    verified: {
      title: 'Компания подтверждена!',
      text: 'Теперь вам доступны все функции платформы: создание заявок, предложений и сделок.',
      action: 'Перейти к работе →',
    },
    documents: {
      title: 'Загрузка документов',
      formats: 'PDF, PNG или JPEG, до 10 МБ.',
      upload: 'Загрузить файл',
      type: {
        charter: 'Устав компании',
        registration_certificate: 'Свидетельство о регистрации (ОГРН/ИНН)',
        address_proof: 'Подтверждение юридического адреса',
        beneficiaries_register: 'Сведения о бенефициарах',
        source_of_funds: 'Подтверждение источника средств',
        other: 'Другое',
      } satisfies Record<KycDocumentType, string>,
    },
    beneficiaries: {
      title: 'Бенефициары',
      empty: 'Укажите хотя бы одного конечного бенефициара.',
      fullName: 'ФИО',
      citizenship: 'Гражданство',
      share: 'Доля, %',
    },
    submit: 'Отправить на проверку',
    submitted: 'Документы отправлены на проверку администратору',
    submitBlocked: 'Загрузите все обязательные документы и укажите хотя бы одного бенефициара.',
  },
  validation: {
    email: 'Некорректный email',
    passwordMin6: 'Минимум 6 символов',
    passwordMin8: 'Минимум 8 символов',
    companyName: 'Укажите название компании',
    countryCode: 'Код страны — 2 буквы (ISO 3166-1 alpha-2)',
    legalAddress: 'Укажите юридический адрес',
    registrationNumber: 'Укажите регистрационный номер',
    borrowerFromRu: 'Заёмщик должен быть из России (RU)',
    lenderNotFromRu: 'Зарубежный кредитор не может быть из России',
    fullName: 'Укажите ФИО',
    citizenship: 'Код страны из двух букв, например RU',
    shareFormat: 'Доля в процентах, например 25 или 12.5',
    shareRange: 'Доля от 0 до 100%',
  },
  errors: {
    VALIDATION_ERROR: 'Проверьте правильность заполнения полей',
    UNAUTHORIZED: 'Требуется вход в систему',
    FORBIDDEN: 'У вас нет прав для этого действия',
    CSRF_INVALID: 'Сессия устарела. Обновите страницу и попробуйте снова',
    KYC_NOT_VERIFIED: 'Сначала пройдите проверку компании',
    NOT_FOUND: 'Запись не найдена',
    CONFLICT: 'Операция невозможна: данные уже изменились',
    INVALID_STATUS_TRANSITION: 'Переход в этот статус невозможен',
    RATE_LIMITED: 'Слишком много запросов. Подождите немного',
    INTERNAL: 'Внутренняя ошибка сервиса',
    NOT_IMPLEMENTED: 'Функция ещё в разработке',
    UNAVAILABLE: 'Сервис временно недоступен',
    unknown: 'Неизвестная ошибка',
    network: 'Ошибка сети',
    generic: 'Что-то пошло не так',
  },
}

export type Dictionary = typeof ru

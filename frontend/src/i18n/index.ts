import { useEffect } from 'react'
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import type { components } from '@/lib/api-schema'
import { en } from './en'
import { ru, type Dictionary } from './ru'

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    resources: { translation: Dictionary }
  }
}

export type Language = 'ru' | 'en'
export const LANGUAGES: Language[] = ['ru', 'en']

const STORAGE_KEY = 'lp.language'
const LOCALE: Record<Language, string> = { ru: 'ru-RU', en: 'en-GB' }

// Выбор пользователя. По умолчанию русский: язык браузера не угадываем,
// переключатель виден на входе и регистрации.
export function chosenLanguage(): Language {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'en' ? 'en' : 'ru'
  } catch {
    return 'ru'
  }
}

export function setLanguage(language: Language): void {
  try {
    localStorage.setItem(STORAGE_KEY, language)
  } catch {
    // Хранилище недоступно (приватный режим) — язык действует до перезагрузки.
  }
  void i18n.changeLanguage(language)
}

export function currentLanguage(): Language {
  return i18n.language === 'en' ? 'en' : 'ru'
}

export type ValidationKey = `validation.${keyof Dictionary['validation']}`

// Схемы zod хранят ключ сообщения, а не текст: перевод — при показе (FieldError),
// поэтому уже показанная ошибка меняет язык вместе с интерфейсом.
export function validationMessage(key: ValidationKey): string {
  return key
}

// Локаль для Intl: даты, числа и суммы.
export function currentLocale(): string {
  return LOCALE[currentLanguage()]
}

type Role = components['schemas']['Role']

// Английский нужен иностранным кредиторам (#102). Заёмщики — российские компании,
// админка не переводится: им всегда русский. До входа (role нет) — выбор со страницы входа.
export function canChooseLanguage(role: Role | undefined): boolean {
  return role !== 'admin' && role !== 'borrower'
}

// Выбор пользователя в этом браузере сохраняется и возвращается после выхода
// заёмщика или админа.
export function useRoleLanguage(role: Role | undefined): void {
  useEffect(() => {
    void i18n.changeLanguage(canChooseLanguage(role) ? chosenLanguage() : 'ru')
  }, [role])
}

i18n.on('languageChanged', (language) => {
  if (typeof document !== 'undefined') document.documentElement.lang = language
})

void i18n.use(initReactI18next).init({
  resources: { ru: { translation: ru }, en: { translation: en } },
  lng: chosenLanguage(),
  fallbackLng: 'ru',
  interpolation: { escapeValue: false },
  initAsync: false,
})

export { i18n }

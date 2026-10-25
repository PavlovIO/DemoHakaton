// Ключ кэша /auth/me. Отдельный модуль: useAuth.tsx экспортирует только компоненты и хуки
// (fast refresh), а мутации, меняющие организацию (KYC), перечитывают /auth/me по нему.
export const ME_KEY = ['me'] as const

// Карточка компании открывается из очереди проверки, модерации заявок, AML-флагов и аудита;
// ссылка «назад» в карточке ведёт туда, откуда её открыли.
const CARD_ORIGINS = {
  kyc: { to: '/admin/kyc', label: 'Очередь проверки' },
  moderation: { to: '/admin/applications', label: 'Модерация заявок' },
  aml: { to: '/admin/aml', label: 'AML-флаги' },
  audit: { to: '/admin/audit', label: 'Аудит' },
}

type CardOrigin = keyof typeof CARD_ORIGINS

// Пропсы <Link> в карточку компании: источник уходит в state истории, а не в URL.
export function companyCardLink(organizationId: string, from: CardOrigin) {
  return { to: `/admin/organizations/${organizationId}`, state: { from } }
}

export const AML_ORGANIZATION_PARAM = 'organization_id'

// Компания пользователя: в аудите UUID автора действия ведёт сюда (user_id → organization_id).
export function companyCardByUserPath(userId: string) {
  return `/admin/organizations?by_user=${userId}`
}

// AML-флаги одной компании: так их открывает карточка компании.
export function companyAmlFlagsPath(organizationId: string) {
  return `/admin/aml?${AML_ORGANIZATION_PARAM}=${organizationId}`
}

// Куда вести «назад» по state истории; без него (прямая ссылка, перезагрузка) — в очередь проверки.
export function companyCardOrigin(state: unknown) {
  const from = (state as { from?: unknown } | null)?.from
  return typeof from === 'string' && from in CARD_ORIGINS ? CARD_ORIGINS[from as CardOrigin] : CARD_ORIGINS.kyc
}

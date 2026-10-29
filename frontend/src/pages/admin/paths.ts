// Карточка компании открывается из очереди проверки, модерации заявок и AML-флагов;
// ссылка «назад» в карточке ведёт туда, откуда её открыли.
const CARD_ORIGINS = {
  kyc: { to: '/admin/kyc', label: 'Очередь проверки' },
  moderation: { to: '/admin/applications', label: 'Модерация заявок' },
  aml: { to: '/admin/aml', label: 'AML-флаги' },
}

type CardOrigin = keyof typeof CARD_ORIGINS

// Пропсы <Link> в карточку компании: источник уходит в state истории, а не в URL.
export function companyCardLink(organizationId: string, from: CardOrigin) {
  return { to: `/admin/organizations/${organizationId}`, state: { from } }
}

// Куда вести «назад» по state истории; без него (прямая ссылка, перезагрузка) — в очередь проверки.
export function companyCardOrigin(state: unknown) {
  const from = (state as { from?: unknown } | null)?.from
  return typeof from === 'string' && from in CARD_ORIGINS ? CARD_ORIGINS[from as CardOrigin] : CARD_ORIGINS.kyc
}

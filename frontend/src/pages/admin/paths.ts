// Карточка компании открывается из очереди проверки, модерации заявок и AML-флагов.
export function companyCardPath(organizationId: string) {
  return `/admin/organizations/${organizationId}`
}

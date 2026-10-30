import type { components } from '@/lib/api-schema'
import { useMyOrganization } from '@/hooks/useKyc'
import { useAuth } from '@/hooks/useAuth'

export type KycStatus = components['schemas']['KycStatus']

// Причина отказа KYC есть только в GET /organizations/me; в /auth/me её нет,
// поэтому при статусе rejected догружаем полный профиль (запрос включён только в этом случае).
// Вынесен в отдельный файл: react-refresh допускает экспорт хуков только из не-компонентных модулей.
export function useDashboardKyc(): { status: KycStatus | null; rejectReason: string | null } {
  const { organization } = useAuth()
  const status = organization?.kyc_status ?? null
  const rejected = status === 'rejected'
  const detailsQuery = useMyOrganization({ enabled: rejected })
  return {
    status,
    rejectReason: rejected ? detailsQuery.data?.kyc_reject_reason ?? null : null,
  }
}

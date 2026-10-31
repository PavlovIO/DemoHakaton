import type { components } from '@/lib/api-schema'
import { useMyOrganization } from '@/hooks/useKyc'
import { useAuth } from '@/hooks/useAuth'

export type KycStatus = components['schemas']['KycStatus']

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
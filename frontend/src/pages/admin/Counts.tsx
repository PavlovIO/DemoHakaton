import { useQueryClient } from '@tanstack/react-query'
import { APPLICATIONS_KEY } from '@/hooks/useApplications'
import { KYC_QUEUE_KEY } from '@/hooks/useKyc'

export function useAdminQueueCounts() {
  const queryClient = useQueryClient()
  const kyc = queryClient.getQueryData<{ total?: number }>(KYC_QUEUE_KEY)
  const moderation = queryClient.getQueryData<{ total?: number }>([...APPLICATIONS_KEY, 'moderation'])
  return {
    kycTotal: kyc?.total ?? 0,
    moderationTotal: moderation?.total ?? 0,
  }
}
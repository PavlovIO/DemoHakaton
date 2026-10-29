import { useKycQueueCount } from '@/hooks/useKyc'
import { useModerationQueueCount } from '@/hooks/useApplications'

const COUNT_HOOKS = {
  kyc: useKycQueueCount,
  moderation: useModerationQueueCount,
}

// Число на иконке пункта меню: сколько ждёт решения администратора. Висит на иконке,
// а не на подписи — подпись меню скрыта до ширины lg (планшет).
export function QueueCountBadge({ queue }: { queue: keyof typeof COUNT_HOOKS }) {
  const { data: total = 0 } = COUNT_HOOKS[queue]()
  if (total === 0) return null
  return (
    <span className="absolute -right-2 -top-2 min-w-4 rounded-full bg-danger px-1 text-center text-[10px] font-semibold leading-4 text-white">
      {total}
    </span>
  )
}

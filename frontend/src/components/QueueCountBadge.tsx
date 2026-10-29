import { useKycQueue, useKycQueueCount } from '@/hooks/useKyc'
import { useModerationQueue, useModerationQueueCount } from '@/hooks/useApplications'

export type AdminQueue = 'kyc' | 'moderation'

const COUNT_HOOKS = {
  kyc: useKycQueueCount,
  moderation: useModerationQueueCount,
}

const LIST_HOOKS = {
  kyc: useKycQueue,
  moderation: useModerationQueue,
}

function TotalBadge({ total = 0 }: { total?: number }) {
  if (total === 0) return null
  return (
    <span className="absolute -right-2 -top-2 min-w-4 rounded-full bg-danger px-1 text-center text-[10px] font-semibold leading-4 text-white">
      {total}
    </span>
  )
}

function CountQueryBadge({ queue }: { queue: AdminQueue }) {
  const { data } = COUNT_HOOKS[queue]()
  return <TotalBadge total={data} />
}

// Читает кэш списка открытой страницы очереди: опрашивает его сама страница.
function QueueListBadge({ queue }: { queue: AdminQueue }) {
  const { data } = LIST_HOOKS[queue]({ observeOnly: true })
  return <TotalBadge total={data?.total} />
}

// Число на иконке пункта меню: сколько ждёт решения администратора. Висит на иконке,
// а не на подписи — подпись меню скрыта до ширины lg (планшет). На странице самой
// очереди её список уже опрашивается и знает total, отдельный счётчик не нужен.
export function QueueCountBadge({ queue, onQueuePage }: { queue: AdminQueue; onQueuePage: boolean }) {
  return onQueuePage ? <QueueListBadge queue={queue} /> : <CountQueryBadge queue={queue} />
}

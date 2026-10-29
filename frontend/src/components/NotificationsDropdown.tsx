import { useState, useEffect, useRef } from 'react'
import { Bell, CheckCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useMarkAllNotificationsRead, useNotifications } from '@/hooks/useNotifications'
import { handleApiError } from '@/lib/api-errors'
import { currentLocale } from '@/i18n'

export function NotificationsDropdown() {
  const { t } = useTranslation()
  const [isOpen, setIsOpen] = useState(false)
  const { data } = useNotifications()
  const notifications = data?.items ?? []
  const unreadCount = data?.unread_total ?? 0
  const markAllRead = useMarkAllNotificationsRead()
  const dropdownRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const markAllAsRead = () => markAllRead.mutate(undefined, { onError: handleApiError })

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="relative rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-700"
      >
        <Bell className="h-5 w-5" />
        {unreadCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-danger text-[10px] font-bold text-white">
            {unreadCount}
          </span>
        )}
      </button>

      {isOpen && (
        <div className="absolute right-0 z-50 mt-2 w-80 rounded-lg border border-gray-200 bg-white shadow-lg">
          <div className="flex items-center justify-between border-b border-gray-200 px-4 py-3">
            <h3 className="text-sm font-semibold text-gray-900">{t('notifications.title')}</h3>
            {unreadCount > 0 && (
              <button
                onClick={markAllAsRead}
                className="inline-flex items-center gap-1 text-xs text-primary-600 hover:text-primary-700"
              >
                <CheckCheck className="h-3 w-3" />
                {t('notifications.markAll')}
              </button>
            )}
          </div>

          <div className="max-h-96 overflow-y-auto">
            {notifications.length === 0 ? (
              <div className="px-4 py-8 text-center text-sm text-gray-500">
                {t('notifications.empty')}
              </div>
            ) : (
              notifications.map((notification) => (
                <div
                  key={notification.id}
                  className={cn(
                    'border-b border-gray-100 px-4 py-3 transition-colors hover:bg-gray-50',
                    !notification.is_read && 'bg-primary-50/30'
                  )}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex-1">
                      <p className="text-sm font-medium text-gray-900">{notification.title}</p>
                    </div>
                    <span className="shrink-0 text-xs text-gray-400">{new Date(notification.created_at).toLocaleTimeString(currentLocale(), { hour: '2-digit', minute: '2-digit' })}</span>
                  </div>
                </div>
              ))
            )}
          </div>

          <div className="border-t border-gray-200 px-4 py-2">
            <button className="w-full text-center text-xs text-primary-600 hover:text-primary-700">
              {t('notifications.showAll')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
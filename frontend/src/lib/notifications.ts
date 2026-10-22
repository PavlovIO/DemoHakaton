export interface Notification {
  id: string
  title: string
  description: string
  type: 'offer' | 'application' | 'deal' | 'system'
  entity_id?: string
  isRead: boolean
  timestamp: string
}

export function addNotification(notif: Omit<Notification, 'id' | 'isRead' | 'timestamp'>) {
  const existing = JSON.parse(localStorage.getItem('mock_notifications') || '[]')
  
  const timeNow = new Date().toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
  
  const newNotif: Notification = {
    ...notif,
    id: Math.random().toString(),
    isRead: false,
    timestamp: timeNow
  }
  
  localStorage.setItem('mock_notifications', JSON.stringify([newNotif, ...existing]))
  
  // Вызываем кастомное событие, чтобы шапка сайта сразу обновилась
  window.dispatchEvent(new Event('notifications-updated'))
}
import { useState, useEffect, useRef } from 'react'
import { Button } from '@/components/ui/Button'
import { addNotification } from '@/lib/notifications'

interface Message {
  id: string
  senderRole: 'borrower' | 'lender' // <-- КТО НАПИСАЛ (а не "я/не я")
  text: string
  created_at: string
}

interface ChatModalProps {
  isOpen: boolean
  onClose: () => void
  offerId: string
  offerNumber: string
  counterpartyName?: string
  role: 'borrower' | 'lender' // <-- КТО СМОТРИТ
}

export function ChatModal({ isOpen, onClose, offerId, offerNumber, counterpartyName, role }: ChatModalProps) {
  const [messages, setMessages] = useState<Message[]>([])
  const [newMessage, setNewMessage] = useState('')
  const messagesEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (isOpen) {
      const saved = JSON.parse(localStorage.getItem(`mock_messages_${offerId}`) || '[]')
      setMessages(saved)
    }
  }, [isOpen, offerId])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (!isOpen) return null

  const handleSend = (e: React.FormEvent) => {
    e.preventDefault()
    if (!newMessage.trim()) return

    const msg: Message = {
      id: Math.random().toString(),
      senderRole: role, // <-- СОХРАНЯЕМ РЕАЛЬНУЮ РОЛЬ
      text: newMessage.trim(),
      created_at: new Date().toISOString()
    }

    const updated = [...messages, msg]
    setMessages(updated)
    localStorage.setItem(`mock_messages_${offerId}`, JSON.stringify(updated))
    setNewMessage('')

    addNotification({
      type: 'system',
      title: `Новое сообщение`,
      description: `Вам написали по офферу ${offerNumber}`,
      entity_id: offerId
    })
    // автоответ УБРАН — на демо переключаемся между аккаунтами
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-lg rounded-lg bg-white shadow-xl flex flex-col h-[600px] max-h-[90vh]">
        
        <div className="flex justify-between items-center p-4 border-b bg-gray-50 rounded-t-lg">
          <div>
            <h3 className="text-lg font-semibold text-gray-900">Чат по офферу</h3>
            <p className="text-xs text-gray-500">Заявка: {offerNumber}</p>
            {counterpartyName && (
              <p className="text-xs text-gray-500 font-medium">Собеседник: {counterpartyName}</p>
            )}
          </div>
          <Button variant="outline" onClick={onClose}>Закрыть</Button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-4 bg-gray-50">
          {messages.length === 0 && (
            <div className="text-center text-gray-500 text-sm mt-10">
              Напишите первое сообщение, чтобы обсудить детали сделки.
            </div>
          )}
          {messages.map((m) => {
            const isMine = m.senderRole === role // <-- СРАВНИВАЕМ РОЛИ
            return (
              <div key={m.id} className={`flex ${isMine ? 'justify-end' : 'justify-start'}`}>
                <div className={`max-w-[80%] rounded-lg p-3 text-sm ${
                  isMine
                    ? 'bg-primary-600 text-white rounded-br-none'
                    : 'bg-white border text-gray-900 rounded-bl-none'
                }`}>
                  <p>{m.text}</p>
                  <span className={`text-[10px] block mt-1 ${isMine ? 'text-primary-200' : 'text-gray-400'}`}>
                    {m.senderRole === 'borrower' ? 'Заемщик' : 'Кредитор'} · {new Date(m.created_at).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })}
                  </span>
                </div>
              </div>
            )
          })}
          <div ref={messagesEndRef} />
        </div>

        <form onSubmit={handleSend} className="p-4 border-t bg-white rounded-b-lg flex gap-2">
          <input
            type="text"
            value={newMessage}
            onChange={e => setNewMessage(e.target.value)}
            placeholder="Напишите сообщение..."
            className="flex-1 rounded-md border-gray-300 shadow-sm p-2 border text-sm focus:border-primary-500 focus:ring-primary-500"
          />
          <Button type="submit">Отправить</Button>
        </form>

      </div>
    </div>
  )
}
import { useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { Button, Spinner, TextField } from './ui'
import type { Conversation, Message, Session } from '../types'

type Props = { session: Session; conversation: Conversation | null; messages: Message[]; isLoading: boolean; isRealtimeConnected: boolean; hasLiveCall: boolean; callBanner?: ReactNode; onStartCall: (conversation: Conversation) => void; onShowMembers: (conversation: Conversation) => void; onSend: (content: string) => Promise<boolean> }
function title(conversation: Conversation) { return conversation.title || conversation.direct_display_name || conversation.direct_username || (conversation.kind === 'direct' ? 'Личный диалог' : conversation.kind) }
function deliveryStatus(message: Message) { return (message.recipient_count ?? 0) > 0 && (message.read_by ?? 0) >= (message.recipient_count ?? 0) ? 'Прочитано' : 'Отправлено' }

function EmptyChat() { return <div className="chat-empty"><div className="chat-empty__mark">✦</div><h1>Разговоры без лишнего шума</h1><p>Выберите существующий диалог или создайте новый в левой панели.</p></div> }

export function ChatPanel({ session, conversation, messages, isLoading, isRealtimeConnected, hasLiveCall, callBanner, onStartCall, onShowMembers, onSend }: Props) {
  const [isSending, setIsSending] = useState(false)
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const content = String(new FormData(form).get('content') ?? '').trim()
    if (!content) return
    setIsSending(true)
    if (await onSend(content)) form.reset()
    setIsSending(false)
  }
  if (!conversation) return <section className="chat"><EmptyChat /></section>
  return <section className="chat"><header className="chat__header"><div className="chat__avatar">{title(conversation).slice(0, 1).toUpperCase()}</div><div><h1>{title(conversation)}</h1><p><span className={isRealtimeConnected ? 'realtime-status realtime-status--live' : 'realtime-status'} />{hasLiveCall ? 'Звонок идёт' : isRealtimeConnected ? 'В сети' : 'Подключаюсь…'}</p></div>{conversation.kind === 'group_chat' ? <Button variant="ghost" className="chat__members" onClick={() => onShowMembers(conversation)}>Участники</Button> : null}<Button variant="secondary" className="chat__call" onClick={() => onStartCall(conversation)}>{hasLiveCall ? 'Войти в звонок' : 'Начать звонок'}</Button></header><div className="chat__call-banner">{callBanner}</div><div className="message-list">{isLoading ? <div className="messages-loading"><Spinner /></div> : null}{!isLoading && messages.length === 0 ? <p className="messages-empty">Здесь пока тихо. Напишите первое сообщение.</p> : null}{messages.map(message => <article className={message.sender_id === session.user.id ? 'message message--mine' : 'message'} key={message.id}><p>{message.content || 'Системное сообщение'}</p><time dateTime={message.created_at}>{new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}{message.sender_id === session.user.id ? <span className="message__status">{deliveryStatus(message)}</span> : null}</time></article>)}</div><form className="composer" onSubmit={submit}><TextField name="content" placeholder="Напишите сообщение" autoComplete="off" /><Button type="submit" disabled={isSending}>{isSending ? '...' : 'Отправить'}</Button></form></section>
}

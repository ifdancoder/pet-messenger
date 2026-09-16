import { useState } from 'react'
import type { FormEvent } from 'react'
import { Button, Spinner, TextField } from './ui'
import type { Conversation, Message, Session } from '../types'

function title(conversation: Conversation) { return conversation.title || conversation.direct_display_name || conversation.direct_username || (conversation.kind === 'direct' ? 'Личный диалог' : conversation.kind) }
function deliveryStatus(message: Message) { return (message.recipient_count ?? 0) > 0 && (message.read_by ?? 0) >= (message.recipient_count ?? 0) ? 'Прочитано' : 'Отправлено' }

function EmptyChat() { return <div className="chat-empty"><div className="chat-empty__mark">✦</div><h1>Разговоры без лишнего шума</h1><p>Выберите существующий диалог или создайте новый в левой панели.</p></div> }

type HeaderProps = { conversation: Conversation | null; isRealtimeConnected: boolean; hasLiveCall: boolean; isInCall: boolean; onStartCall: (conversation: Conversation) => void; onShowMembers: (conversation: Conversation) => void }
// ChatHeader is its own top-level component, not a child of ChatBody's own
// <section> — a docked call needs to sit between the two, spanning the
// workspace's full width, which a single shared container would have
// constrained to the chat's own narrow reading width along with it.
export function ChatHeader({ conversation, isRealtimeConnected, hasLiveCall, isInCall, onStartCall, onShowMembers }: HeaderProps) {
  if (!conversation) return null
  return <header className="chat__header"><div className="chat__avatar">{title(conversation).slice(0, 1).toUpperCase()}</div><div><h1>{title(conversation)}</h1><p><span className={isRealtimeConnected ? 'realtime-status realtime-status--live' : 'realtime-status'} />{hasLiveCall ? 'Звонок идёт' : isRealtimeConnected ? 'В сети' : 'Подключаюсь…'}</p></div>{conversation.kind === 'group_chat' ? <Button variant="ghost" className="chat__members" onClick={() => onShowMembers(conversation)}>Участники</Button> : null}{isInCall ? null : <Button variant="secondary" className="chat__call" onClick={() => onStartCall(conversation)}>{hasLiveCall ? 'Войти в звонок' : 'Начать звонок'}</Button>}</header>
}

type BodyProps = { session: Session; conversation: Conversation | null; messages: Message[]; isLoading: boolean; onSend: (content: string) => Promise<boolean> }
export function ChatBody({ session, conversation, messages, isLoading, onSend }: BodyProps) {
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
  // message-list itself spans the full width (so its background texture
  // reaches the edges), but the messages inside stay in a centered, capped
  // inner column — otherwise a left-aligned received bubble and a
  // right-aligned sent one would anchor to opposite edges of the full-width
  // container and end up far apart, the same gap narrowing the column used
  // to fix in the first place.
  return <section className="chat"><div className="message-list"><div className="message-list__inner">{isLoading ? <div className="messages-loading"><Spinner /></div> : null}{!isLoading && messages.length === 0 ? <p className="messages-empty">Здесь пока тихо. Напишите первое сообщение.</p> : null}{messages.map(message => <article className={message.sender_id === session.user.id ? 'message message--mine' : 'message'} key={message.id}><p>{message.content || 'Системное сообщение'}</p><time dateTime={message.created_at}>{new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}{message.sender_id === session.user.id ? <span className="message__status">{deliveryStatus(message)}</span> : null}</time></article>)}</div></div><form className="composer" onSubmit={submit}><div className="composer__inner"><TextField name="content" placeholder="Напишите сообщение" autoComplete="off" /><Button type="submit" disabled={isSending}>{isSending ? '...' : 'Отправить'}</Button></div></form></section>
}

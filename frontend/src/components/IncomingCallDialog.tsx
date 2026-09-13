import { Button } from './ui'
import type { Conversation } from '../types'

type Props = { conversation: Conversation; onAccept: () => void; onDismiss: () => void }
function caller(conversation: Conversation) {
  if (conversation.kind !== 'direct') return `В группе «${conversation.title || 'Без названия'}»`
  const name = conversation.direct_display_name || conversation.direct_username || 'Собеседник'
  return conversation.direct_username ? `${name} · @${conversation.direct_username}` : name
}
function PhoneIcon() { return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7.2 3.5 4.7 4.8c-.8.4-1.2 1.3-.9 2.2 1.9 6.3 6.8 11.2 13.1 13.1.9.3 1.8-.1 2.2-.9l1.3-2.5c.3-.7.1-1.5-.5-1.9l-3-1.7a1.6 1.6 0 0 0-1.8.1l-1.4 1.1a12.8 12.8 0 0 1-4-4l1.1-1.4c.5-.5.5-1.3.1-1.8l-1.7-3a1.5 1.5 0 0 0-2-.6Z" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg> }
export function IncomingCallDialog({ conversation, onAccept, onDismiss }: Props) {
  return <section className="incoming-call" role="dialog" aria-modal="true" aria-label="Входящий звонок"><div className="incoming-call__pulse"><PhoneIcon /></div><span>Входящий звонок</span><h2>Вам звонит</h2><p>{caller(conversation)}</p><div><Button variant="danger" onClick={onDismiss}>Отклонить</Button><Button onClick={onAccept}>Принять</Button></div></section>
}

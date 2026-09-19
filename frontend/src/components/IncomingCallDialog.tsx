import { Avatar } from './Avatar'
import type { Conversation } from '../types'

function PhoneIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7.2 3.5 4.7 4.8c-.8.4-1.2 1.3-.9 2.2 1.9 6.3 6.8 11.2 13.1 13.1.9.3 1.8-.1 2.2-.9l1.3-2.5c.3-.7.1-1.5-.5-1.9l-3-1.7a1.6 1.6 0 0 0-1.8.1l-1.4 1.1a12.8 12.8 0 0 1-4-4l1.1-1.4c.5-.5.5-1.3.1-1.8l-1.7-3a1.5 1.5 0 0 0-2-.6Z" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
}

function CameraIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4.5 7.5h10A2.5 2.5 0 0 1 17 10v4a2.5 2.5 0 0 1-2.5 2.5h-10A2.5 2.5 0 0 1 2 14v-4a2.5 2.5 0 0 1 2.5-2.5ZM17 10l4-2.3v8.6L17 14" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
}

function identity(conversation: Conversation) {
  if (conversation.kind === 'direct') {
    const name = conversation.direct_display_name || conversation.direct_username || 'Собеседник'
    return { name, detail: conversation.direct_username ? `@${conversation.direct_username}` : 'Личный звонок', avatarUrl: conversation.direct_avatar_url }
  }
  return { name: conversation.title || 'Групповой звонок', detail: 'Звонок в группе', avatarUrl: conversation.avatar_url }
}

type Props = { conversation: Conversation; token: string; onAccept: (withCamera: boolean) => void; onDismiss: () => void }

export function IncomingCallDialog({ conversation, token, onAccept, onDismiss }: Props) {
  const caller = identity(conversation)
  return <section className="incoming-call" role="dialog" aria-modal="true" aria-label="Входящий звонок">
    <span className="incoming-call__eyebrow">Входящий звонок</span>
    <Avatar subject={{ avatarUrl: caller.avatarUrl, displayName: caller.name }} token={token} className="incoming-call__avatar" />
    <div className="incoming-call__identity"><h2>{caller.name}</h2><p>{caller.detail}</p></div>
    <div className="incoming-call__actions">
      <button type="button" className="incoming-call__decline" onClick={onDismiss}>Отклонить</button>
      <button type="button" className="incoming-call__accept" onClick={() => onAccept(false)} aria-label="Принять аудиозвонок" data-tooltip="Принять аудиозвонок"><PhoneIcon /></button>
      <button type="button" className="incoming-call__accept" onClick={() => onAccept(true)} aria-label="Принять с камерой" data-tooltip="Принять с камерой"><CameraIcon /></button>
    </div>
  </section>
}

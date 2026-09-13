import { Button } from './ui'
import type { Conversation } from '../types'

type Props = { conversation: Conversation; onAccept: () => void; onDismiss: () => void }
function caller(conversation: Conversation) {
  if (conversation.kind !== 'direct') return `В группе «${conversation.title || 'Без названия'}»`
  const name = conversation.direct_display_name || conversation.direct_username || 'Собеседник'
  return conversation.direct_username ? `${name} · @${conversation.direct_username}` : name
}
export function IncomingCallDialog({ conversation, onAccept, onDismiss }: Props) {
  return <section className="incoming-call" role="dialog" aria-modal="true" aria-label="Входящий звонок"><div className="incoming-call__pulse">⌁</div><span>Входящий звонок</span><h2>Вам звонит</h2><p>{caller(conversation)}</p><div><Button variant="danger" onClick={onDismiss}>Отклонить</Button><Button onClick={onAccept}>Принять</Button></div></section>
}

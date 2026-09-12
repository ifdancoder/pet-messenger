import { useState } from 'react'
import { Brand } from './Brand'
import { NewConversationForm } from './NewConversationForm'
import { Button, IconButton, Spinner } from './ui'
import type { Conversation, Session } from '../types'

type Props = {
  session: Session
  conversations: Conversation[]
  activeConversation: Conversation | null
  isLoading: boolean
  onSelect: (conversation: Conversation) => void
  onCreate: (kind: 'direct' | 'group', value: string) => Promise<boolean>
  onLogout: () => void
}

function conversationTitle(conversation: Conversation) { return conversation.title || (conversation.kind === 'direct' ? 'Личный диалог' : conversation.kind) }

export function ConversationSidebar({ session, conversations, activeConversation, isLoading, onSelect, onCreate, onLogout }: Props) {
  const [isComposerOpen, setIsComposerOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  async function copyID() {
    await navigator.clipboard.writeText(session.user.id).catch(() => undefined)
    setCopied(true); window.setTimeout(() => setCopied(false), 1_800)
  }
  return <aside className="sidebar">
    <header className="sidebar__header"><Brand /><IconButton label="Выйти" onClick={onLogout}>↗</IconButton></header>
    <section className="identity"><div className="identity__avatar">{(session.user.display_name || session.user.username).slice(0, 1).toUpperCase()}</div><div><strong>{session.user.display_name || session.user.username}</strong><span>@{session.user.username}</span></div><Button variant="ghost" className="identity__copy" onClick={() => void copyID()}>{copied ? 'Скопировано' : 'Копировать мой ID'}</Button></section>
    <div className="sidebar__section-title"><h2>Диалоги</h2><Button variant="secondary" className="button--compact" onClick={() => setIsComposerOpen(value => !value)}>Новый</Button></div>
    {isComposerOpen ? <NewConversationForm onCreate={onCreate} /> : null}
    <nav className="conversation-list" aria-label="Диалоги">
      {isLoading ? <div className="sidebar-loading"><Spinner /></div> : null}
      {!isLoading && conversations.length === 0 ? <p className="sidebar-empty">Начните разговор, создав личный диалог или группу.</p> : null}
      {conversations.map(conversation => <button className={conversation.id === activeConversation?.id ? 'conversation-item conversation-item--active' : 'conversation-item'} key={conversation.id} onClick={() => onSelect(conversation)}><span className="conversation-item__avatar">{conversationTitle(conversation).slice(0, 1).toUpperCase()}</span><span><b>{conversationTitle(conversation)}</b><small>{conversation.kind === 'direct' ? 'личный разговор' : 'группа'}</small></span></button>)}
    </nav>
  </aside>
}

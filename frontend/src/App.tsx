import { useCallback, useState } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { ChatPanel } from './components/ChatPanel'
import { ConversationSidebar } from './components/ConversationSidebar'
import { Notice } from './components/ui'
import { useMessenger } from './hooks/useMessenger'
import { clearSession, readSession, saveSession } from './lib/session'
import type { Session } from './types'
import './App.css'

function Messenger({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const messenger = useMessenger(session, onLogout)
  return <main className="messenger-app"><ConversationSidebar session={session} conversations={messenger.conversations} activeConversation={messenger.activeConversation} isLoading={messenger.isLoadingConversations} onSelect={messenger.selectConversation} onCreate={messenger.createConversation} onLogout={onLogout} /><div className="workspace">{messenger.error ? <Notice>{messenger.error}</Notice> : null}<ChatPanel session={session} conversation={messenger.activeConversation} messages={messenger.messages} isLoading={messenger.isLoadingMessages} onSend={messenger.sendMessage} /></div></main>
}

export default function App() {
  const [session, setSession] = useState<Session | null>(readSession)
  const authenticate = useCallback((session: Session) => { saveSession(session); setSession(session) }, [])
  const logout = useCallback(() => { clearSession(); setSession(null) }, [])
  return session ? <Messenger session={session} onLogout={logout} /> : <AuthScreen onAuthenticated={authenticate} />
}

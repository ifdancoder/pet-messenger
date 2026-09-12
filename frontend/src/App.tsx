import { useCallback, useEffect, useState } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { CallOverlay } from './components/CallOverlay'
import { ChatPanel } from './components/ChatPanel'
import { ConversationSidebar } from './components/ConversationSidebar'
import { Notice } from './components/ui'
import { useMessenger } from './hooks/useMessenger'
import { useCall } from './hooks/useCall'
import { clearSession, readSession, saveSession } from './lib/session'
import type { Conversation, Session } from './types'
import './App.css'

function Messenger({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const messenger = useMessenger(session, onLogout)
  const { start, close, shareScreen, localStream, remoteStreams, error: callError, isConnecting, isOpen } = useCall(session)
  const [callConversation, setCallConversation] = useState<Conversation | null>(null)
  useEffect(() => { if (callConversation) void Promise.resolve().then(() => start(callConversation)) }, [callConversation, start])
  const endCall = useCallback(async () => { await close(true); setCallConversation(null) }, [close])
  return <main className="messenger-app"><ConversationSidebar session={session} conversations={messenger.conversations} activeConversation={messenger.activeConversation} isLoading={messenger.isLoadingConversations} onSelect={messenger.selectConversation} onCreate={messenger.createConversation} onLogout={onLogout} /><div className="workspace">{messenger.error ? <Notice>{messenger.error}</Notice> : null}<ChatPanel session={session} conversation={messenger.activeConversation} messages={messenger.messages} isLoading={messenger.isLoadingMessages} isRealtimeConnected={messenger.isRealtimeConnected} onStartCall={setCallConversation} onSend={messenger.sendMessage} />{callConversation ? <CallOverlay localStream={localStream} remoteStreams={remoteStreams} error={callError} isConnecting={isConnecting && !isOpen} onShare={shareScreen} onEnd={endCall} /> : null}</div></main>
}

export default function App() {
  const [session, setSession] = useState<Session | null>(readSession)
  const authenticate = useCallback((session: Session) => { saveSession(session); setSession(session) }, [])
  const logout = useCallback(() => { clearSession(); setSession(null) }, [])
  return session ? <Messenger session={session} onLogout={logout} /> : <AuthScreen onAuthenticated={authenticate} />
}

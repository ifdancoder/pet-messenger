import { useCallback, useEffect, useRef, useState } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { CallOverlay } from './components/CallOverlay'
import { IncomingCallDialog } from './components/IncomingCallDialog'
import { ChatPanel } from './components/ChatPanel'
import { ConversationSidebar } from './components/ConversationSidebar'
import { MembersPanel } from './components/MembersPanel'
import { Notice } from './components/ui'
import { useMessenger } from './hooks/useMessenger'
import { useCall } from './hooks/useCall'
import { useLiveCalls } from './hooks/useLiveCall'
import { clearSession, readSession, saveSession } from './lib/session'
import { playTone } from './lib/sounds'
import type { Conversation, Session } from './types'
import './App.css'

function Messenger({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const messenger = useMessenger(session, onLogout)
  const { start, close, shareScreen, toggleMute, toggleCamera, moderate, localStream, remoteStreams, peers, error: callError, isConnecting, isOpen, isMuted, isCameraOff } = useCall(session)
  const [callConversation, setCallConversation] = useState<Conversation | null>(null)
  const [membersConversation, setMembersConversation] = useState<Conversation | null>(null)
  const liveCalls = useLiveCalls(session, messenger.conversations)
  const ringingCallID = useRef<string | null>(null)
  const [dismissedCallID, setDismissedCallID] = useState<string | null>(null)
  const activeLiveCall = liveCalls.find(item => item.conversation.id === messenger.activeConversation?.id)
  const incomingCall = liveCalls.find(item => item.call.initiator_id !== session.user.id && item.call.id !== dismissedCallID)
  useEffect(() => {
    if (!incomingCall || callConversation || ringingCallID.current === incomingCall.call.id) return
    ringingCallID.current = incomingCall.call.id
    playTone('call')
  }, [callConversation, incomingCall])
  useEffect(() => { if (callConversation) void Promise.resolve().then(() => start(callConversation)) }, [callConversation, start])
  const endCall = useCallback(async () => { await close(true); setCallConversation(null) }, [close])
  return <main className="messenger-app"><ConversationSidebar session={session} conversations={messenger.conversations} unreadByConversation={messenger.unreadByConversation} activeConversation={messenger.activeConversation} isLoading={messenger.isLoadingConversations} onSelect={messenger.selectConversation} onCreate={messenger.createConversation} onLogout={onLogout} /><div className="workspace">{messenger.error ? <Notice>{messenger.error}</Notice> : null}<ChatPanel session={session} conversation={messenger.activeConversation} messages={messenger.messages} isLoading={messenger.isLoadingMessages} isRealtimeConnected={messenger.isRealtimeConnected} hasLiveCall={Boolean(activeLiveCall)} onStartCall={setCallConversation} onShowMembers={setMembersConversation} onSend={messenger.sendMessage} />{membersConversation ? <MembersPanel conversation={membersConversation} session={session} onClose={() => setMembersConversation(null)} /> : null}{incomingCall && !callConversation ? <IncomingCallDialog conversation={incomingCall.conversation} onAccept={() => setCallConversation(incomingCall.conversation)} onDismiss={() => setDismissedCallID(incomingCall.call.id)} /> : null}{callConversation ? <CallOverlay localStream={localStream} remoteStreams={remoteStreams} peers={peers} currentUserID={session.user.id} canModerate={callConversation.kind !== 'direct'} error={callError} isConnecting={isConnecting} isConnected={isOpen} isMuted={isMuted} isCameraOff={isCameraOff} onShare={shareScreen} onToggleMute={toggleMute} onToggleCamera={toggleCamera} onModerate={moderate} onEnd={endCall} /> : null}</div></main>
}

export default function App() {
  const [session, setSession] = useState<Session | null>(readSession)
  const authenticate = useCallback((session: Session) => { saveSession(session); setSession(session) }, [])
  const logout = useCallback(() => { clearSession(); setSession(null) }, [])
  return session ? <Messenger session={session} onLogout={logout} /> : <AuthScreen onAuthenticated={authenticate} />
}

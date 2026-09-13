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
import { api } from './lib/api'
import type { Conversation, Session } from './types'
import './App.css'

function Messenger({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const messenger = useMessenger(session, onLogout)
  const { start, close, shareScreen, toggleMute, toggleCamera, moderate, localStream, remoteStreams, peers, error: callError, isConnecting, isOpen, isMuted, isCameraOff, isSharingScreen } = useCall(session)
  const [callConversation, setCallConversation] = useState<Conversation | null>(null)
  const [membersConversation, setMembersConversation] = useState<Conversation | null>(null)
  const [callParticipantNames, setCallParticipantNames] = useState<Record<string, string>>({})
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
  useEffect(() => {
    if (!callConversation) return
    let current = true
    void api.members(callConversation.id, session.access_token)
      .then(({ members }) => { if (current) setCallParticipantNames(Object.fromEntries(members.map(member => [member.user_id, `@${member.username} · ${member.display_name || member.username}`]))) })
      .catch(() => { if (current) setCallParticipantNames({}) })
    return () => { current = false }
  }, [callConversation, session.access_token])
  const openCall = useCallback((conversation: Conversation) => { setCallParticipantNames({}); setCallConversation(conversation) }, [])
  const endCall = useCallback(async () => { await close(true); setCallParticipantNames({}); setCallConversation(null) }, [close])
  const ownCallName = `@${session.user.username} · ${session.user.display_name || session.user.username}`
  return <main className="messenger-app"><ConversationSidebar session={session} conversations={messenger.conversations} unreadByConversation={messenger.unreadByConversation} activeConversation={messenger.activeConversation} isLoading={messenger.isLoadingConversations} onSelect={messenger.selectConversation} onCreate={messenger.createConversation} onLogout={onLogout} /><div className="workspace">{messenger.error ? <Notice>{messenger.error}</Notice> : null}<ChatPanel session={session} conversation={messenger.activeConversation} messages={messenger.messages} isLoading={messenger.isLoadingMessages} isRealtimeConnected={messenger.isRealtimeConnected} hasLiveCall={Boolean(activeLiveCall)} onStartCall={openCall} onShowMembers={setMembersConversation} onSend={messenger.sendMessage} />{membersConversation ? <MembersPanel conversation={membersConversation} session={session} onClose={() => setMembersConversation(null)} /> : null}{incomingCall && !callConversation ? <IncomingCallDialog conversation={incomingCall.conversation} onAccept={() => openCall(incomingCall.conversation)} onDismiss={() => setDismissedCallID(incomingCall.call.id)} /> : null}{callConversation ? <CallOverlay localStream={localStream} remoteStreams={remoteStreams} peers={peers} currentUserID={session.user.id} participantNames={{ ...callParticipantNames, [session.user.id]: ownCallName }} canModerate={callConversation.kind !== 'direct'} error={callError} isConnecting={isConnecting} isConnected={isOpen} isMuted={isMuted} isCameraOff={isCameraOff} isSharingScreen={isSharingScreen} onShare={shareScreen} onToggleMute={toggleMute} onToggleCamera={toggleCamera} onModerate={moderate} onEnd={endCall} /> : null}</div></main>
}

export default function App() {
  const [session, setSession] = useState<Session | null>(readSession)
  const authenticate = useCallback((session: Session) => { saveSession(session); setSession(session) }, [])
  const logout = useCallback(() => { clearSession(); setSession(null) }, [])
  return session ? <Messenger session={session} onLogout={logout} /> : <AuthScreen onAuthenticated={authenticate} />
}

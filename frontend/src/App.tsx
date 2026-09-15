import { useCallback, useEffect, useRef, useState } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { CallOverlay } from './components/CallOverlay'
import { IncomingCallDialog } from './components/IncomingCallDialog'
import { ChatPanel } from './components/ChatPanel'
import { ConversationSidebar } from './components/ConversationSidebar'
import { MembersPanel } from './components/MembersPanel'
import { SettingsModal } from './components/SettingsModal'
import { Notice } from './components/ui'
import { useMessenger } from './hooks/useMessenger'
import { useCall } from './hooks/useCall'
import { useLiveCalls } from './hooks/useLiveCall'
import { clearSession, readSession, saveSession } from './lib/session'
import { playTone } from './lib/sounds'
import { api, setSessionRefreshHandler } from './lib/api'
import type { Conversation, Session } from './types'
import './App.css'

function Messenger({ session, onLogout, onSessionUpdate }: { session: Session; onLogout: () => void; onSessionUpdate: (session: Session) => void }) {
  const messenger = useMessenger(session, onLogout)
  const { start, close, shareScreen, stopScreenShare, toggleMute, toggleCamera, switchDevice, setNoiseSuppression, moderate, reconnectNow, localStream, localScreenStream, remoteStreams, peers, error: callError, isConnecting, isOpen, isMuted, isCameraOff, isSharingScreen, reconnectExhausted } = useCall(session)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [callConversation, setCallConversation] = useState<Conversation | null>(null)
  // manuallyMinimized is the "collapse it myself" override; leaving the
  // call's own conversation minimizes automatically (see `minimized` below)
  // without needing this at all, but a user parked right there wants to be
  // able to shrink it too — for instance to read the chat behind it.
  const [manuallyMinimized, setManuallyMinimized] = useState(false)
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
  const openCall = useCallback((conversation: Conversation) => { setCallParticipantNames({}); setManuallyMinimized(false); setCallConversation(conversation) }, [])
  const endCall = useCallback(async () => { await close(true); setCallParticipantNames({}); setCallConversation(null); setManuallyMinimized(false) }, [close])
  const ownCallName = `@${session.user.username} · ${session.user.display_name || session.user.username}`
  // Navigating to a different chat than the one the call belongs to shrinks
  // it to a popup automatically — the call itself (useCall, above) was
  // already independent of which conversation is on screen, so this only
  // ever changes what CallOverlay renders, never anything about the
  // connection. onRestore both un-minimizes and jumps back to that
  // conversation, so expanding a call you navigated away from doesn't leave
  // you staring at an unrelated chat with a full-screen overlay on top of it.
  const minimized = Boolean(callConversation) && (manuallyMinimized || messenger.activeConversation?.id !== callConversation?.id)
  const restoreCall = useCallback(() => {
    setManuallyMinimized(false)
    if (callConversation) messenger.selectConversation(callConversation)
  }, [callConversation, messenger])
  return <main className="messenger-app"><ConversationSidebar session={session} conversations={messenger.conversations} unreadByConversation={messenger.unreadByConversation} activeConversation={messenger.activeConversation} isLoading={messenger.isLoadingConversations} onSelect={messenger.selectConversation} onCreate={messenger.createConversation} onLogout={onLogout} onOpenSettings={() => setSettingsOpen(true)} /><div className="workspace">{messenger.error ? <Notice>{messenger.error}</Notice> : null}<ChatPanel session={session} conversation={messenger.activeConversation} messages={messenger.messages} isLoading={messenger.isLoadingMessages} isRealtimeConnected={messenger.isRealtimeConnected} hasLiveCall={Boolean(activeLiveCall)} onStartCall={openCall} onShowMembers={setMembersConversation} onSend={messenger.sendMessage} />{membersConversation ? <MembersPanel conversation={membersConversation} session={session} onClose={() => setMembersConversation(null)} /> : null}{incomingCall && !callConversation ? <IncomingCallDialog conversation={incomingCall.conversation} onAccept={() => openCall(incomingCall.conversation)} onDismiss={() => setDismissedCallID(incomingCall.call.id)} /> : null}{callConversation ? <CallOverlay localStream={localStream} localScreenStream={localScreenStream} remoteStreams={remoteStreams} peers={peers} currentUserID={session.user.id} participantNames={{ ...callParticipantNames, [session.user.id]: ownCallName }} canModerate={callConversation.kind !== 'direct'} error={callError} isConnecting={isConnecting} isConnected={isOpen} isMuted={isMuted} isCameraOff={isCameraOff} isSharingScreen={isSharingScreen} reconnectExhausted={reconnectExhausted} minimized={minimized} onToggleMinimize={() => setManuallyMinimized(current => !current)} onRestore={restoreCall} onReconnect={reconnectNow} onShare={shareScreen} onStopShare={stopScreenShare} onToggleMute={toggleMute} onToggleCamera={toggleCamera} onModerate={moderate} onEnd={endCall} /> : null}{settingsOpen ? <SettingsModal session={session} onSessionUpdate={onSessionUpdate} onClose={() => setSettingsOpen(false)} isCallActive={isOpen} switchDevice={switchDevice} setNoiseSuppression={setNoiseSuppression} /> : null}</div></main>
}

export default function App() {
  const [session, setSession] = useState<Session | null>(readSession)
  const authenticate = useCallback((session: Session) => { saveSession(session); setSession(session) }, [])
  const logout = useCallback(() => {
    const refreshToken = readSession()?.refresh_token
    clearSession(); setSession(null)
    if (refreshToken) void api.logout(refreshToken).catch(() => undefined)
  }, [])
  // A 401 anywhere silently refreshes the token; this keeps the in-memory
  // session (and everything derived from it, like useCall's WS reconnects)
  // in sync with what api.ts just wrote to storage — or logs out if the
  // refresh token itself was rejected.
  useEffect(() => {
    setSessionRefreshHandler(refreshed => refreshed ? setSession(refreshed) : setSession(null))
    return () => setSessionRefreshHandler(null)
  }, [])
  return session ? <Messenger session={session} onLogout={logout} onSessionUpdate={authenticate} /> : <AuthScreen onAuthenticated={authenticate} />
}

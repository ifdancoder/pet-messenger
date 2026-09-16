import { useCallback, useEffect, useRef, useState } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { CallAudio, CallOverlay, DockedCallBar, MiniCallBar } from './components/CallOverlay'
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
  // fullScreen is the "expanded" override for the call's own conversation:
  // starts true (a call opens full-screen), the full view's own minimize
  // button clears it (see dockCall), and the docked bar's expand button
  // sets it back. Navigating away from the call's conversation floats it
  // regardless of this value (see `floating` below), so it only matters
  // while parked on that exact chat.
  const [fullScreen, setFullScreen] = useState(true)
  const [membersConversation, setMembersConversation] = useState<Conversation | null>(null)
  const [callParticipantNames, setCallParticipantNames] = useState<Record<string, string>>({})
  // volumes and connectedSince are lifted here rather than owned by
  // whichever view (full/docked/floating) happens to be mounted, so a
  // per-participant volume tweak or the call's own elapsed timer survives
  // switching between them instead of silently resetting whenever the
  // visible view unmounts.
  const [volumes, setVolumes] = useState<Record<string, number>>({})
  const [connectedSince, setConnectedSince] = useState<number | null>(null)
  useEffect(() => {
    if (isOpen) setConnectedSince(current => current ?? Date.now())
    else if (!isConnecting) setConnectedSince(null)
  }, [isOpen, isConnecting])
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
  const openCall = useCallback((conversation: Conversation) => { setCallParticipantNames({}); setFullScreen(true); setCallConversation(conversation) }, [])
  const endCall = useCallback(async () => { await close(true); setCallParticipantNames({}); setCallConversation(null); setFullScreen(true) }, [close])
  const ownCallName = `@${session.user.username} · ${session.user.display_name || session.user.username}`
  // Three mutually exclusive views for an active call: full-screen (on its
  // own chat, fullScreen), docked (on its own chat, not fullScreen — a
  // compact bar above that chat's messages), and floating (navigated away
  // entirely — a small popup over everything, regardless of fullScreen).
  const onCallChat = Boolean(callConversation) && messenger.activeConversation?.id === callConversation?.id
  const floating = Boolean(callConversation) && !onCallChat
  const docked = Boolean(callConversation) && onCallChat && !fullScreen
  // dockCall both navigates to the call's own conversation and drops out of
  // full-screen — used by the floating popup's click (never full-screen:
  // returning to a chat you were reading something else in shouldn't cover
  // it) and by the full view's own minimize button (already on that
  // conversation, so the navigation there is a no-op).
  const dockCall = useCallback(() => {
    setFullScreen(false)
    if (callConversation) messenger.selectConversation(callConversation)
  }, [callConversation, messenger])
  const expandCall = useCallback(() => setFullScreen(true), [])
  const setVolume = useCallback((key: string, volume: number) => setVolumes(previous => ({ ...previous, [key]: volume })), [])
  const otherCallNames = peers.filter(peer => peer.user_id !== session.user.id).map(peer => callParticipantNames[peer.user_id] ?? 'Собеседник')
  return <main className="messenger-app"><ConversationSidebar session={session} conversations={messenger.conversations} unreadByConversation={messenger.unreadByConversation} activeConversation={messenger.activeConversation} isLoading={messenger.isLoadingConversations} onSelect={messenger.selectConversation} onCreate={messenger.createConversation} onLogout={onLogout} onOpenSettings={() => setSettingsOpen(true)} /><div className="workspace">{messenger.error ? <Notice>{messenger.error}</Notice> : null}<ChatPanel session={session} conversation={messenger.activeConversation} messages={messenger.messages} isLoading={messenger.isLoadingMessages} isRealtimeConnected={messenger.isRealtimeConnected} hasLiveCall={Boolean(activeLiveCall)} callBanner={docked ? <DockedCallBar names={otherCallNames} isMuted={isMuted} isCameraOff={isCameraOff} isSharingScreen={isSharingScreen} connectedSince={connectedSince} onToggleMute={toggleMute} onToggleCamera={toggleCamera} onShare={shareScreen} onStopShare={stopScreenShare} onExpand={expandCall} onEnd={endCall} /> : null} onStartCall={openCall} onShowMembers={setMembersConversation} onSend={messenger.sendMessage} />{membersConversation ? <MembersPanel conversation={membersConversation} session={session} onClose={() => setMembersConversation(null)} /> : null}{incomingCall && !callConversation ? <IncomingCallDialog conversation={incomingCall.conversation} onAccept={() => openCall(incomingCall.conversation)} onDismiss={() => setDismissedCallID(incomingCall.call.id)} /> : null}{callConversation ? <CallAudio remoteStreams={remoteStreams} peers={peers} volumes={volumes} floating={floating} /> : null}{floating ? <MiniCallBar names={otherCallNames} isMuted={isMuted} onToggleMute={toggleMute} onEnd={() => void endCall()} onExpand={dockCall} connectedSince={connectedSince} /> : null}{callConversation && !floating && !docked ? <CallOverlay localStream={localStream} localScreenStream={localScreenStream} remoteStreams={remoteStreams} peers={peers} currentUserID={session.user.id} participantNames={{ ...callParticipantNames, [session.user.id]: ownCallName }} canModerate={callConversation.kind !== 'direct'} error={callError} isConnecting={isConnecting} isConnected={isOpen} isMuted={isMuted} isCameraOff={isCameraOff} isSharingScreen={isSharingScreen} reconnectExhausted={reconnectExhausted} volumes={volumes} onVolumeChange={setVolume} onDock={dockCall} onReconnect={reconnectNow} onShare={shareScreen} onStopShare={stopScreenShare} onToggleMute={toggleMute} onToggleCamera={toggleCamera} onModerate={moderate} onEnd={endCall} /> : null}{settingsOpen ? <SettingsModal session={session} onSessionUpdate={onSessionUpdate} onClose={() => setSettingsOpen(false)} isCallActive={isOpen} switchDevice={switchDevice} setNoiseSuppression={setNoiseSuppression} /> : null}</div></main>
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

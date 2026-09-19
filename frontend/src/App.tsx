import { useCallback, useEffect, useRef, useState } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { CallAudio, CallOverlay, MiniCallBar } from './components/CallOverlay'
import { CallPreviewBar } from './components/CallPreviewBar'
import { IncomingCallDialog } from './components/IncomingCallDialog'
import { ChatBody, ChatHeader } from './components/ChatPanel'
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
import type { Conversation, MemberProfile, Session } from './types'
import './App.css'

function Messenger({ session, onLogout, onSessionUpdate }: { session: Session; onLogout: () => void; onSessionUpdate: (session: Session) => void }) {
  const messenger = useMessenger(session, onLogout)
  const { start, close, shareScreen, stopScreenShare, toggleMute, toggleCamera, switchDevice, setNoiseSuppression, moderate, reconnectNow, localStream, localScreenStream, remoteStreams, peers, error: callError, isConnecting, isOpen, isMuted, isCameraOff, isSharingScreen, reconnectExhausted } = useCall(session)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [callConversation, setCallConversation] = useState<Conversation | null>(null)
  const [joinWithCamera, setJoinWithCamera] = useState(false)
  const [callMembers, setCallMembers] = useState<MemberProfile[]>([])
  // fullScreen is the "expanded" override for the call's own conversation:
  // starts true (a call opens full-screen), the full view's own minimize
  // button clears it (see dockCall), and the docked bar's expand button
  // sets it back. Navigating away from the call's conversation floats it
  // regardless of this value (see `floating` below), so it only matters
  // while parked on that exact chat.
  const [fullScreen, setFullScreen] = useState(true)
  const [membersConversation, setMembersConversation] = useState<Conversation | null>(null)
  const [callParticipantNames, setCallParticipantNames] = useState<Record<string, string>>({})
  const [callParticipantAvatars, setCallParticipantAvatars] = useState<Record<string, string>>({})
  // volumes is lifted here rather than owned by whichever view
  // (full/docked/floating) happens to be mounted, so a per-participant
  // volume tweak survives switching between them instead of silently
  // resetting whenever the visible view unmounts.
  const [volumes, setVolumes] = useState<Record<string, number>>({})
  const { calls: liveCalls, hasLoadedOnce: liveCallsLoaded } = useLiveCalls(session, messenger.conversations, messenger.isLoadingConversations)
  const ringingCallID = useRef<string | null>(null)
  const [dismissedCallID, setDismissedCallID] = useState<string | null>(null)
  const activeLiveCall = liveCalls.find(item => item.conversation.id === messenger.activeConversation?.id)
  // The preview bar (not yet joined) and the real docked/full call view are
  // mutually exclusive: once callConversation matches the viewed
  // conversation, the real CallOverlay (with its own docked mode) takes
  // over this same visual slot.
  const showPreviewBar = Boolean(activeLiveCall) && callConversation?.id !== messenger.activeConversation?.id
  // Edge-triggered, not level-triggered: knownCallIDs is seeded from the
  // very first REAL poll (whatever's already live when this component
  // mounts — including right after a reload — never rings), and only a
  // call ID that appears on a LATER poll without having been in the
  // previous one counts as "just started ringing for me". A poll-derived
  // level check (just "is there a live call I haven't dismissed") used to
  // resurrect the dialog for an already-ongoing call on every reload.
  //
  // "Very first REAL poll" needs its own readiness signal, not tick
  // counting: useLiveCalls itself is told to skip fetching entirely while
  // messenger.isLoadingConversations is true (see its `skip` param), so it
  // never runs a placeholder fetch against a not-yet-real conversations
  // list. liveCallsLoaded flips true exactly once, at the hook's first real
  // fetch — so there's exactly one unambiguous "first snapshot" here to
  // seed from, and no risk of a genuinely new call that starts during the
  // loading window being silently folded into the baseline.
  const knownCallIDs = useRef<Set<string> | null>(null)
  const [justStartedCallID, setJustStartedCallID] = useState<string | null>(null)
  useEffect(() => {
    if (!liveCallsLoaded) return
    const currentIDs = new Set(liveCalls.map(item => item.call.id))
    if (knownCallIDs.current === null) {
      knownCallIDs.current = currentIDs
      return
    }
    const newlyAppeared = liveCalls.find(item => item.call.initiator_id !== session.user.id && !knownCallIDs.current!.has(item.call.id))
    if (newlyAppeared) setJustStartedCallID(newlyAppeared.call.id)
    knownCallIDs.current = currentIDs
  }, [liveCalls, session.user.id, liveCallsLoaded])
  const incomingCall = liveCalls.find(item => item.call.id === justStartedCallID && item.call.id !== dismissedCallID)
  // The elapsed-time source of truth is the call record's own started_at
  // (polled every 2.5s by useLiveCalls), not a client-local Date.now()
  // snapshot: a local snapshot made each participant's timer start from
  // when THEIR OWN connection happened to open (drifting between
  // participants) and reset to zero on any refresh even though the call
  // itself kept running. Looked up via callConversation rather than
  // messenger.activeConversation so it stays correct while floating.
  const currentLiveCall = callConversation ? liveCalls.find(item => item.conversation.id === callConversation.id) : undefined
  const connectedSince = currentLiveCall ? new Date(currentLiveCall.call.started_at).getTime() : null
  useEffect(() => {
    if (!incomingCall || callConversation || ringingCallID.current === incomingCall.call.id) return
    ringingCallID.current = incomingCall.call.id
    playTone('call')
  }, [callConversation, incomingCall])
  useEffect(() => { if (callConversation) void Promise.resolve().then(() => start(callConversation, joinWithCamera)) }, [callConversation, joinWithCamera, start])
  useEffect(() => {
    if (!callConversation) return
    let current = true
    void api.members(callConversation.id, session.access_token)
      .then(({ members }) => {
        if (!current) return
        setCallMembers(members)
        setCallParticipantNames(Object.fromEntries(members.map(member => [member.user_id, `@${member.username} · ${member.display_name || member.username}`])))
        setCallParticipantAvatars(Object.fromEntries(members.filter(member => member.avatar_url).map(member => [member.user_id, member.avatar_url])))
      })
      .catch(() => { if (current) { setCallMembers([]); setCallParticipantNames({}); setCallParticipantAvatars({}) } })
    return () => { current = false; setCallMembers([]) }
  }, [callConversation, session.access_token])
  const openCall = useCallback((conversation: Conversation, withCamera = false) => { setCallMembers([]); setCallParticipantNames({}); setCallParticipantAvatars({}); setJoinWithCamera(withCamera); setFullScreen(true); setCallConversation(conversation) }, [])
  const endCall = useCallback(async () => { await close(true); setCallMembers([]); setCallParticipantNames({}); setCallParticipantAvatars({}); setJoinWithCamera(false); setCallConversation(null); setFullScreen(true) }, [close])
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
  // toggleDock flips fullScreen without navigating anywhere — used by the
  // full/docked view's own header button, since it's only ever clicked
  // while already on the call's conversation.
  const toggleDock = useCallback(() => setFullScreen(current => !current), [])
  const setVolume = useCallback((key: string, volume: number) => setVolumes(previous => ({ ...previous, [key]: volume })), [])
  const otherCallNames = peers.filter(peer => peer.user_id !== session.user.id).map(peer => callParticipantNames[peer.user_id] ?? 'Собеседник')
  // Docked and full-screen are the exact same CallOverlay, just mounted in
  // two different places (see the docked= prop) — sharing one props object
  // keeps the two call sites from drifting out of sync on everything but
  // that flag.
  const callOverlayProps = callConversation ? {
    localStream, localScreenStream, remoteStreams, peers,
    currentUserID: session.user.id,
    token: session.access_token,
    participantNames: { ...callParticipantNames, [session.user.id]: ownCallName },
    participantAvatars: session.user.avatar_url ? { ...callParticipantAvatars, [session.user.id]: session.user.avatar_url } : callParticipantAvatars,
    canModerate: callConversation.kind !== 'direct',
    waitingParticipants: callMembers.filter(member => member.user_id !== session.user.id && !peers.some(peer => peer.user_id === member.user_id)).map(member => ({ userID: member.user_id, name: `@${member.username} · ${member.display_name || member.username}`, avatarUrl: member.avatar_url })),
    error: callError, isConnecting, isConnected: isOpen, isMuted, isCameraOff, isSharingScreen, reconnectExhausted, connectedSince,
    volumes, onVolumeChange: setVolume, onToggleDock: toggleDock,
    onReconnect: reconnectNow, onShare: shareScreen, onStopShare: stopScreenShare,
    onToggleMute: toggleMute, onToggleCamera: toggleCamera, onModerate: moderate, onEnd: endCall,
  } : null
  return <main className="messenger-app">
    <ConversationSidebar session={session} conversations={messenger.conversations} unreadByConversation={messenger.unreadByConversation} activeConversation={messenger.activeConversation} isLoading={messenger.isLoadingConversations} onSelect={messenger.selectConversation} onCreate={messenger.createConversation} onLogout={onLogout} onOpenSettings={() => setSettingsOpen(true)} />
    <div className="workspace">
      {messenger.error ? <Notice>{messenger.error}</Notice> : null}
      <ChatHeader conversation={messenger.activeConversation} token={session.access_token} isRealtimeConnected={messenger.isRealtimeConnected} hasLiveCall={Boolean(activeLiveCall)} isInCall={onCallChat} onStartCall={openCall} onShowMembers={setMembersConversation} />
      {showPreviewBar && activeLiveCall ? <CallPreviewBar liveCall={activeLiveCall} session={session} participantNames={callParticipantNames} participantAvatars={callParticipantAvatars} onJoin={withCamera => openCall(activeLiveCall.conversation, withCamera)} /> : null}
      {callOverlayProps && docked ? <div className="call-docked-wrapper"><CallOverlay {...callOverlayProps} docked /></div> : null}
      <ChatBody session={session} conversation={messenger.activeConversation} messages={messenger.messages} isLoading={messenger.isLoadingMessages} onSend={messenger.sendMessage} />
      {membersConversation ? <MembersPanel conversation={membersConversation} session={session} onClose={() => setMembersConversation(null)} /> : null}
      {incomingCall && !callConversation ? <IncomingCallDialog conversation={incomingCall.conversation} token={session.access_token} onAccept={withCamera => openCall(incomingCall.conversation, withCamera)} onDismiss={() => setDismissedCallID(incomingCall.call.id)} /> : null}
      {callConversation ? <CallAudio remoteStreams={remoteStreams} peers={peers} volumes={volumes} floating={floating} /> : null}
      {floating ? <MiniCallBar names={otherCallNames} isMuted={isMuted} onToggleMute={toggleMute} onEnd={() => void endCall()} onExpand={dockCall} connectedSince={connectedSince} /> : null}
      {callOverlayProps && !floating && !docked ? <CallOverlay {...callOverlayProps} docked={false} /> : null}
      {settingsOpen ? <SettingsModal session={session} onSessionUpdate={onSessionUpdate} onClose={() => setSettingsOpen(false)} isCallActive={isOpen} switchDevice={switchDevice} setNoiseSuppression={setNoiseSuppression} /> : null}
    </div>
  </main>
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

import { useEffect } from 'react'
import { CallOverlay } from './CallOverlay'
import { useCallPreview } from '../hooks/useCallPreview'
import type { LiveCall } from '../hooks/useLiveCall'
import type { Session } from '../types'

type Props = {
  liveCall: LiveCall
  session: Session
  participantNames: Record<string, string>
  participantAvatars: Record<string, string>
  onJoin: (withCamera: boolean) => void
}

// Before accepting, render the actual docked CallOverlay — not a visual
// approximation of it. The only altered controls are the two green accept
// actions; when accepted, App replaces this with the very same component
// carrying the user's real media connection.
export function CallPreviewBar({ liveCall, session, participantNames, participantAvatars, onJoin }: Props) {
  const preview = useCallPreview(session)
  useEffect(() => {
    preview.start(liveCall.call.id)
    return () => preview.stop()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveCall.call.id])

  // A viewer session itself appears in the room roster after preview.start.
  // The pre-accept dock must show who initiated the call, not the recipient
  // or a transient viewer/observer entry that happens to share the room.
  const peers = liveCall.participants
    .filter(participant => participant.user_id === liveCall.call.initiator_id)
    .map(participant => ({ ...participant, tracks: [] }))
  const conversation = liveCall.conversation
  const initiatorName = conversation.kind === 'direct'
    ? `@${conversation.direct_username || conversation.direct_display_name || 'Собеседник'} · ${conversation.direct_display_name || conversation.direct_username || 'Собеседник'}`
    : participantNames[liveCall.call.initiator_id] ?? 'Инициатор звонка'
  const names = { ...participantNames, [liveCall.call.initiator_id]: initiatorName }
  const avatars = conversation.kind === 'direct' && conversation.direct_avatar_url
    ? { ...participantAvatars, [liveCall.call.initiator_id]: conversation.direct_avatar_url }
    : participantAvatars
  const initiatorStreams = preview.remoteStreams.filter(stream => stream.ownerID === liveCall.call.initiator_id)

  return <div className="call-docked-wrapper call-preview-bar">
    <CallOverlay
      localStream={null}
      localScreenStream={null}
      remoteStreams={initiatorStreams}
      peers={peers}
      currentUserID={session.user.id}
      token={session.access_token}
      participantNames={names}
      participantAvatars={avatars}
      canModerate={false}
      waitingParticipants={[]}
      incomingActions={{ onAcceptAudio: () => onJoin(false), onAcceptCamera: () => onJoin(true) }}
      showLocal={false}
      error=""
      isConnecting={false}
      isConnected
      isMuted={false}
      isCameraOff
      isSharingScreen={false}
      reconnectExhausted={false}
      connectedSince={new Date(liveCall.call.started_at).getTime()}
      volumes={{}}
      onVolumeChange={() => {}}
      docked
      onToggleDock={() => {}}
      onReconnect={() => {}}
      onShare={async () => {}}
      onStopShare={() => {}}
      onToggleMute={() => {}}
      onToggleCamera={async () => {}}
      onModerate={async () => {}}
      onEnd={async () => {}}
    />
  </div>
}

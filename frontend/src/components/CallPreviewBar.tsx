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

  const peers = liveCall.participants.map(participant => ({ ...participant, tracks: [] }))
  const names = { ...participantNames, [session.user.id]: `@${session.user.username} · ${session.user.display_name || session.user.username}` }
  const avatars = session.user.avatar_url ? { ...participantAvatars, [session.user.id]: session.user.avatar_url } : participantAvatars

  return <div className="call-docked-wrapper call-preview-bar">
    <CallOverlay
      localStream={null}
      localScreenStream={null}
      remoteStreams={preview.remoteStreams}
      peers={peers}
      currentUserID={session.user.id}
      token={session.access_token}
      participantNames={names}
      participantAvatars={avatars}
      canModerate={false}
      waitingParticipants={[]}
      incomingActions={{ onAcceptAudio: () => onJoin(false), onAcceptCamera: () => onJoin(true) }}
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

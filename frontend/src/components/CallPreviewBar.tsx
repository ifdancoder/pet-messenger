import { useEffect, useState } from 'react'
import { CallOverlay } from './CallOverlay'
import { useCallPreview } from '../hooks/useCallPreview'
import { api } from '../lib/api'
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
  const [memberNames, setMemberNames] = useState<Record<string, string>>({})
  const [memberAvatars, setMemberAvatars] = useState<Record<string, string>>({})
  useEffect(() => {
    preview.start(liveCall.call.id)
    return () => preview.stop()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveCall.call.id])

  // The live-call endpoint intentionally returns only media state, not a
  // profile payload. Load group profiles independently so a real member is
  // never rendered as a made-up "Собеседник N" while the preview is open.
  useEffect(() => {
    if (liveCall.conversation.kind === 'direct') return
    let current = true
    void api.members(liveCall.conversation.id, session.access_token)
      .then(({ members }) => {
        if (!current) return
        setMemberNames(Object.fromEntries(members.map(member => [member.user_id, `@${member.username} · ${member.display_name || member.username}`])))
        setMemberAvatars(Object.fromEntries(members.filter(member => member.avatar_url).map(member => [member.user_id, member.avatar_url])))
      })
      .catch(() => { if (current) { setMemberNames({}); setMemberAvatars({}) } })
    return () => { current = false }
  }, [liveCall.conversation.id, liveCall.conversation.kind, session.access_token])

  // A viewer session itself appears in the room roster after preview.start.
  // Show every real participant still in the room, never the current user.
  // In particular, this must not be tied to the original initiator: after
  // they leave, either side still needs a docked way back into the live room
  // for as long as its solo timeout has not ended it.
  // Preview sockets are intentionally visible to the SFU roster for media
  // delivery, but they are not accepted call participants and must never be
  // rendered as one in another person's incoming docked preview.
  const visibleParticipants = liveCall.participants.filter(participant => participant.connected && participant.user_id !== session.user.id && participant.role !== 'viewer')
  const participantIDs = new Set(visibleParticipants.map(participant => participant.user_id))
  const peers = liveCall.participants
    .filter(participant => participantIDs.has(participant.user_id))
    .map(participant => ({ ...participant, tracks: [] }))
  const conversation = liveCall.conversation
  const directPeerID = visibleParticipants[0]?.user_id
  const directPeerName = conversation.kind === 'direct'
    ? `@${conversation.direct_username || conversation.direct_display_name || 'Собеседник'} · ${conversation.direct_display_name || conversation.direct_username || 'Собеседник'}`
    : undefined
  const names = directPeerID && directPeerName
    ? { ...participantNames, ...memberNames, [directPeerID]: directPeerName }
    : { ...participantNames, ...memberNames }
  const avatars = conversation.kind === 'direct' && conversation.direct_avatar_url
    ? { ...participantAvatars, ...memberAvatars, ...(directPeerID ? { [directPeerID]: conversation.direct_avatar_url } : {}) }
    : { ...participantAvatars, ...memberAvatars }
  const participantStreams = preview.remoteStreams.filter(stream => stream.ownerID !== undefined && participantIDs.has(stream.ownerID))

  return <div className="call-docked-wrapper call-preview-bar">
    <CallOverlay
      localStream={null}
      localScreenStream={null}
      remoteStreams={participantStreams}
      peers={peers}
      currentUserID={session.user.id}
      token={session.access_token}
      participantNames={names}
      participantAvatars={avatars}
      canModerate={false}
      waitingParticipants={[]}
      incomingActions={{ onAcceptAudio: () => onJoin(false), onAcceptCamera: () => onJoin(true) }}
      showLocal={false}
      showSpeaking={false}
      error=""
      isConnecting={false}
      isConnected
      isMuted={false}
      isCameraOff
      isSharingScreen={false}
      reconnectExhausted={false}
      // This is an invitation, not a joined call for this account. Showing
      // the global call clock here made it look as if the recipient had
      // already connected.
      connectedSince={null}
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

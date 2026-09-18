import { useEffect, useRef } from 'react'
import { Avatar } from './Avatar'
import { useCallPreview } from '../hooks/useCallPreview'
import type { LiveCall } from '../hooks/useLiveCall'
import type { LiveParticipant } from '../lib/api'
import type { Session } from '../types'

function PhoneIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7.2 3.5 4.7 4.8c-.8.4-1.2 1.3-.9 2.2 1.9 6.3 6.8 11.2 13.1 13.1.9.3 1.8-.1 2.2-.9l1.3-2.5c.3-.7.1-1.5-.5-1.9l-3-1.7a1.6 1.6 0 0 0-1.8.1l-1.4 1.1a12.8 12.8 0 0 1-4-4l1.1-1.4c.5-.5.5-1.3.1-1.8l-1.7-3a1.5 1.5 0 0 0-2-.6Z" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
}

// A tile is "still ringing" (gets the shake/pulse treatment) while it has no
// published camera/screen track and no mic activity of its own to show for
// it — i.e. we have no evidence they've actually connected media yet, only
// that the join endpoint accepted them into the roster.
function isStillRinging(participant: LiveParticipant) {
  return participant.track_kinds.length === 0
}

// A tiny local hook, kept out of the tile's own render logic, purely for the
// imperative srcObject assignment — the same pattern VideoTile in
// CallOverlay.tsx uses.
function useVideoRef(stream?: MediaStream) {
  const ref = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (ref.current) ref.current.srcObject = stream ?? null }, [stream])
  return ref
}

function ParticipantPreviewTile({ participant, name, avatarUrl, token, videoStream }: { participant: LiveParticipant; name: string; avatarUrl?: string; token: string; videoStream?: MediaStream }) {
  const video = useVideoRef(videoStream)
  return <div className={`call-preview__tile${isStillRinging(participant) ? ' call-preview__tile--ringing' : ''}`}>
    {videoStream
      ? <video ref={video} autoPlay playsInline muted />
      : <Avatar subject={{ avatarUrl, displayName: name }} token={token} className="call-preview__avatar" />}
    <span className="call-preview__name">{name}</span>
  </div>
}

type Props = {
  liveCall: LiveCall
  session: Session
  participantNames: Record<string, string>
  participantAvatars: Record<string, string>
  onJoin: () => void
}

export function CallPreviewBar({ liveCall, session, participantNames, participantAvatars, onJoin }: Props) {
  const preview = useCallPreview(session)
  useEffect(() => {
    preview.start(liveCall.call.id)
    return () => preview.stop()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveCall.call.id])

  const streamForOwner = (userID: string) => preview.remoteStreams.find(s => s.ownerID === userID)?.stream

  return <div className="call-preview-bar">
    <div className="call-preview__participants">
      {liveCall.participants.map(participant => (
        <ParticipantPreviewTile
          key={participant.user_id}
          participant={participant}
          name={participantNames[participant.user_id] ?? 'Участник'}
          avatarUrl={participantAvatars[participant.user_id]}
          token={session.access_token}
          videoStream={streamForOwner(participant.user_id)}
        />
      ))}
    </div>
    <button type="button" className="call-preview__join" aria-label="Войти в звонок" onClick={onJoin}>
      <PhoneIcon />
    </button>
  </div>
}

import { useEffect, useRef } from 'react'
import { Avatar } from './Avatar'
import { useCallPreview } from '../hooks/useCallPreview'
import type { LiveCall } from '../hooks/useLiveCall'
import type { Session } from '../types'

function PhoneIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7.2 3.5 4.7 4.8c-.8.4-1.2 1.3-.9 2.2 1.9 6.3 6.8 11.2 13.1 13.1.9.3 1.8-.1 2.2-.9l1.3-2.5c.3-.7.1-1.5-.5-1.9l-3-1.7a1.6 1.6 0 0 0-1.8.1l-1.4 1.1a12.8 12.8 0 0 1-4-4l1.1-1.4c.5-.5.5-1.3.1-1.8l-1.7-3a1.5 1.5 0 0 0-2-.6Z" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
}

function CameraIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4.5 7.5h10A2.5 2.5 0 0 1 17 10v4a2.5 2.5 0 0 1-2.5 2.5h-10A2.5 2.5 0 0 1 2 14v-4a2.5 2.5 0 0 1 2.5-2.5ZM17 10l4-2.3v8.6L17 14" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
}

function useVideoRef(stream?: MediaStream) {
  const ref = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (ref.current) ref.current.srcObject = stream ?? null }, [stream])
  return ref
}

function titleFor(liveCall: LiveCall) {
  const conversation = liveCall.conversation
  return conversation.kind === 'direct'
    ? conversation.direct_display_name || conversation.direct_username || 'Собеседник'
    : conversation.title || 'Групповой звонок'
}

function avatarFor(liveCall: LiveCall) {
  return liveCall.conversation.kind === 'direct' ? liveCall.conversation.direct_avatar_url : liveCall.conversation.avatar_url
}

function ParticipantPreviewTile({ name, avatarUrl, token, videoStream }: { name: string; avatarUrl?: string; token: string; videoStream?: MediaStream }) {
  const video = useVideoRef(videoStream)
  return <article className="call-participant call-preview__participant" data-media-count={videoStream ? 1 : 0}>
    <header><span className="call-participant__identity-text">{name}</span></header>
    <div className="call-participant__media">
      {videoStream
        ? <figure className="call-video"><video ref={video} autoPlay playsInline muted /></figure>
        : <figure className="call-video call-video--identity"><Avatar subject={{ avatarUrl, displayName: name }} token={token} className="identity-tile__mark" /></figure>}
    </div>
  </article>
}

type Props = {
  liveCall: LiveCall
  session: Session
  participantNames: Record<string, string>
  participantAvatars: Record<string, string>
  onJoin: (withCamera: boolean) => void
}

// This is deliberately the same docked-call shell as CallOverlay, not a
// separate notification strip. The recipient sees the people already in the
// call in the familiar layout; only the controls differ until they join.
export function CallPreviewBar({ liveCall, session, participantNames, participantAvatars, onJoin }: Props) {
  const preview = useCallPreview(session)
  useEffect(() => {
    preview.start(liveCall.call.id)
    return () => preview.stop()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveCall.call.id])

  const streamForOwner = (userID: string) => preview.remoteStreams.find(stream => stream.ownerID === userID)?.stream
  const fallbackName = titleFor(liveCall)
  const fallbackAvatar = avatarFor(liveCall)

  return <section className="call-overlay call-overlay--docked call-preview-bar" aria-label="Входящий звонок">
    <header><span className="call-label">Звонок</span></header>
    <div className="call-tile-wrap call-preview__tiles">
      <div className="call-tile-track">
        {liveCall.participants.map(participant => <ParticipantPreviewTile
          key={participant.user_id}
          name={participantNames[participant.user_id] ?? fallbackName}
          avatarUrl={participantAvatars[participant.user_id] ?? fallbackAvatar}
          token={session.access_token}
          videoStream={streamForOwner(participant.user_id)}
        />)}
      </div>
    </div>
    <footer>
      <div className="call-overlay__controls">
        <button type="button" className="call-preview__accept" aria-label="Принять аудиозвонок" data-tooltip="Принять аудиозвонок" onClick={() => onJoin(false)}><PhoneIcon /></button>
        <button type="button" className="call-preview__accept" aria-label="Принять с камерой" data-tooltip="Принять с камерой" onClick={() => onJoin(true)}><CameraIcon /></button>
      </div>
    </footer>
  </section>
}

import { useEffect, useRef, useState } from 'react'
import type { CallPeer, RemoteStream } from '../hooks/useCall'
import { Button, Notice, Spinner } from './ui'

type StreamEntry = Pick<RemoteStream, 'id' | 'stream' | 'trackIDs'>
type TileProps = { stream: MediaStream; muted?: boolean; label: string; focused: boolean; onFocus: () => void }
function isScreenShare(entry: StreamEntry, tracks?: CallPeer['tracks']) {
  const label = tracks?.find(track => entry.trackIDs.includes(track.track_id))?.label
  return label ? label === 'screen' : entry.stream.id.toLowerCase().includes('screen')
}
function FocusIcon({ focused }: { focused: boolean }) {
  return focused
    ? <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3v6H3M15 3v6h6M3 15h6v6M21 15h-6v6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
    : <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3H3v6M15 3h6v6M3 15v6h6M21 15v6h-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
function CameraOffIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h10a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2Zm12 4 5-3v10l-5-3M3 3l18 18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
function VideoTile({ stream, muted, label, focused, onFocus }: TileProps) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  return <figure className="call-video"><video ref={video} autoPlay playsInline muted={muted} /><figcaption>{label}</figcaption><button className="call-video__focus" type="button" aria-label={focused ? 'Свернуть видео' : 'Развернуть видео'} title={focused ? 'Свернуть' : 'Развернуть'} onClick={onFocus}><FocusIcon focused={focused} /></button></figure>
}
function EmptyVideoTile({ label }: { label: string }) {
  return <figure className="call-video call-video--placeholder"><div className="call-video__placeholder"><CameraOffIcon /></div><figcaption>{label}</figcaption></figure>
}
function ParticipantSection({ title, streams, tracks, muted, focusedID, onFocus }: { title: string; streams: StreamEntry[]; tracks?: CallPeer['tracks']; muted?: boolean; focusedID: string | null; onFocus: (id: string) => void }) {
  const videoStreams = streams.filter(entry => entry.stream.getVideoTracks().length > 0).sort((left, right) => Number(isScreenShare(left, tracks)) - Number(isScreenShare(right, tracks)))
  const mediaStatus = videoStreams.length > 1 ? 'видео и демонстрация' : videoStreams.length === 1 ? 'медиа' : 'камера выключена'
  return <article className="call-participant"><header><span>{title}</span><small>{mediaStatus}</small></header><div className="call-participant__media">{videoStreams.length ? videoStreams.map(entry => <VideoTile key={entry.id} stream={entry.stream} muted={muted} label={isScreenShare(entry, tracks) ? 'Демонстрация экрана' : 'Камера'} focused={focusedID === entry.id} onFocus={() => onFocus(entry.id)} />) : <EmptyVideoTile label={title} />}</div></article>
}

type Props = { localStream: MediaStream | null; remoteStreams: RemoteStream[]; peers: CallPeer[]; currentUserID: string; participantNames: Record<string, string>; canModerate: boolean; error: string; isConnecting: boolean; isConnected: boolean; isMuted: boolean; isCameraOff: boolean; onShare: () => Promise<void>; onToggleMute: () => void; onToggleCamera: () => void; onModerate: (userID: string, action: 'mute' | 'unmute' | 'remove') => Promise<void>; onEnd: () => Promise<void> }
export function CallOverlay({ localStream, remoteStreams, peers, currentUserID, participantNames, canModerate, error, isConnecting, isConnected, isMuted, isCameraOff, onShare, onToggleMute, onToggleCamera, onModerate, onEnd }: Props) {
  const [focusedStreamID, setFocusedStreamID] = useState<string | null>(null)
  const otherPeers = peers.filter(peer => peer.user_id !== currentUserID)
  const title = isConnected ? 'Соединение установлено' : isConnecting ? 'Подключаемся…' : 'Соединение потеряно'
  const local = localStream ? [{ id: 'local', stream: localStream, trackIDs: [] }] : []
  const nameFor = (userID: string, fallback: string) => participantNames[userID] ?? fallback
  const sections = new Map<string, StreamEntry[]>()
  for (const stream of remoteStreams) {
    const owner = stream.ownerID ?? otherPeers.find(peer => peer.tracks?.some(track => stream.trackIDs.includes(track.track_id)))?.user_id
    const key = owner ?? `unknown:${stream.id}`
    sections.set(key, [...(sections.get(key) ?? []), stream])
  }
  for (const peer of otherPeers) {
    if (!sections.has(peer.user_id)) sections.set(peer.user_id, [])
  }
  const allStreams = [...local, ...[...sections.values()].flat()]
  const focused = focusedStreamID ? allStreams.find(stream => stream.id === focusedStreamID) : undefined
  const toggleFocus = (id: string) => setFocusedStreamID(current => current === id ? null : id)
  const labelFor = (id: string) => {
    if (id === 'local') return nameFor(currentUserID, 'Вы')
    const owner = [...sections.entries()].find(([, streams]) => streams.some(stream => stream.id === id))?.[0]
    return owner ? nameFor(owner, 'Собеседник') : 'Собеседник'
  }
  const isLocal = (id: string) => id === 'local'
  return <section className="call-overlay" aria-label="Звонок"><header><div><span className="call-label">Звонок</span><h2>{title}</h2></div><Button variant="danger" onClick={() => void onEnd()}>Завершить</Button></header>{error ? <Notice>{error}</Notice> : null}{focused ? <div className="call-focused"><div className="call-focused__main"><VideoTile stream={focused.stream} muted={isLocal(focused.id)} label={labelFor(focused.id)} focused onFocus={() => toggleFocus(focused.id)} /></div><div className="call-focused__strip">{allStreams.filter(stream => stream.id !== focused.id).map(stream => <VideoTile key={stream.id} stream={stream.stream} muted={isLocal(stream.id)} label={labelFor(stream.id)} focused={false} onFocus={() => toggleFocus(stream.id)} />)}</div></div> : <div className="call-grid"><ParticipantSection title={nameFor(currentUserID, 'Вы')} streams={local} muted focusedID={focusedStreamID} onFocus={toggleFocus} />{[...sections].map(([userID, streams], index) => <ParticipantSection key={userID} title={nameFor(userID, `Собеседник ${index + 1}`)} streams={streams} tracks={peers.find(peer => peer.user_id === userID)?.tracks} focusedID={focusedStreamID} onFocus={toggleFocus} />)}{isConnecting ? <div className="call-waiting"><Spinner /><span>Настраиваем защищённое соединение</span></div> : null}{isConnected && otherPeers.length === 0 ? <div className="call-waiting">Ожидаем собеседника</div> : null}</div>}<footer><Button variant="secondary" onClick={onToggleMute}>{isMuted ? 'Включить микрофон' : 'Выключить микрофон'}</Button><Button variant="secondary" onClick={onToggleCamera}>{isCameraOff ? 'Включить камеру' : 'Выключить камеру'}</Button><Button variant="secondary" onClick={() => void onShare()}>Показать экран</Button>{canModerate ? otherPeers.map(peer => <div className="call-peer" key={peer.user_id}><span>{nameFor(peer.user_id, 'Участник')} · {peer.muted ? 'без звука' : 'со звуком'}</span><Button variant="ghost" onClick={() => void onModerate(peer.user_id, peer.muted ? 'unmute' : 'mute')}>{peer.muted ? 'Включить звук' : 'Выключить звук'}</Button><Button variant="danger" onClick={() => void onModerate(peer.user_id, 'remove')}>Исключить</Button></div>) : null}</footer></section>
}

import { useEffect, useRef, useState } from 'react'
import type { CallPeer, RemoteStream, ScreenShareOptions } from '../hooks/useCall'
import { screenFrameRates, screenResolutions } from '../hooks/useCall'
import { Button, Notice, Spinner } from './ui'

type StreamEntry = Pick<RemoteStream, 'id' | 'stream' | 'sourceStreamID'>
type TileProps = { stream: MediaStream; muted?: boolean; label: string; focused: boolean; onFocus: () => void }
// Matches on the MediaStream id the track arrived on, never on the track id:
// a browser mints MediaStreamTrack.id fresh on receipt, so it never equals
// whatever id the SFU put in the SDP — only the stream id survives the trip.
function isScreenShare(entry: StreamEntry, tracks?: CallPeer['tracks']) {
  return tracks?.find(track => track.stream_id === entry.sourceStreamID)?.label === 'screen'
}
function FocusIcon({ focused }: { focused: boolean }) {
  return focused
    ? <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3v6H3M15 3v6h6M3 15h6v6M21 15h-6v6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
    : <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3H3v6M15 3h6v6M3 15v6h6M21 15v6h-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
function VideoTile({ stream, muted, label, focused, onFocus }: TileProps) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  return <figure className="call-video"><video ref={video} autoPlay playsInline muted={muted} /><figcaption>{label}</figcaption><button className="call-video__focus" type="button" aria-label={focused ? 'Свернуть видео' : 'Развернуть видео'} title={focused ? 'Свернуть' : 'Развернуть'} onClick={onFocus}><FocusIcon focused={focused} /></button></figure>
}
function EmptyVideoTile({ label }: { label: string }) {
  const name = label.split(' · ').at(-1) || label
  return <figure className="call-video call-video--placeholder"><div className="call-video__profile"><span className="call-video__avatar" aria-hidden="true">{name.slice(0, 1).toUpperCase()}</span><figcaption>{label}</figcaption></div></figure>
}
function ParticipantSection({ title, streams, tracks, muted, focusedID, onFocus }: { title: string; streams: StreamEntry[]; tracks?: CallPeer['tracks']; muted?: boolean; focusedID: string | null; onFocus: (id: string) => void }) {
  const videoStreams = streams.filter(entry => entry.stream.getVideoTracks().length > 0).sort((left, right) => Number(isScreenShare(left, tracks)) - Number(isScreenShare(right, tracks)))
  const mediaStatus = videoStreams.length > 1 ? 'видео и демонстрация' : videoStreams.length === 1 ? 'медиа' : 'камера выключена'
  return <article className="call-participant"><header><span>{title}</span><small>{mediaStatus}</small></header><div className="call-participant__media">{videoStreams.length ? videoStreams.map(entry => <VideoTile key={entry.id} stream={entry.stream} muted={muted} label={isScreenShare(entry, tracks) ? 'Демонстрация экрана' : 'Камера'} focused={focusedID === entry.id} onFocus={() => onFocus(entry.id)} />) : <EmptyVideoTile label={title} />}</div></article>
}
// A remote participant's mic (and a shared screen's own tab/system audio)
// arrives as its own audio-only MediaStream, never bundled into a video
// tile's srcObject — video tiles come and go with the grid/focused layout,
// but audio must keep playing regardless of which tiles happen to be
// mounted right now. Rendered off-screen, once per audio track, always.
function AudioSink({ stream }: { stream: MediaStream }) {
  const audio = useRef<HTMLAudioElement>(null)
  useEffect(() => { if (audio.current) audio.current.srcObject = stream }, [stream])
  return <audio ref={audio} autoPlay hidden />
}
function ShareScreenModal({ onConfirm, onCancel }: { onConfirm: (options: ScreenShareOptions) => void; onCancel: () => void }) {
  const [resolutionIndex, setResolutionIndex] = useState(1)
  const [frameRate, setFrameRate] = useState(30)
  const [audio, setAudio] = useState(true)
  const resolution = screenResolutions[resolutionIndex]
  return <div className="share-modal-backdrop" onClick={onCancel}><section className="share-modal" role="dialog" aria-modal="true" aria-label="Настройки демонстрации экрана" onClick={event => event.stopPropagation()}>
    <h2>Демонстрация экрана</h2>
    <label className="share-modal__field"><span>Разрешение</span><select value={resolutionIndex} onChange={event => setResolutionIndex(Number(event.target.value))}>{screenResolutions.map((option, index) => <option key={option.label} value={index}>{option.label}</option>)}</select></label>
    <label className="share-modal__field"><span>Частота кадров</span><select value={frameRate} onChange={event => setFrameRate(Number(event.target.value))}>{screenFrameRates.map(fps => <option key={fps} value={fps}>{fps} кадр/с</option>)}</select></label>
    <label className="share-modal__toggle"><input type="checkbox" checked={audio} onChange={event => setAudio(event.target.checked)} />Транслировать звук экрана</label>
    <div className="share-modal__actions"><Button variant="secondary" onClick={onCancel}>Отмена</Button><Button onClick={() => onConfirm({ width: resolution.width, height: resolution.height, frameRate, audio })}>Начать демонстрацию</Button></div>
  </section></div>
}

type Props = { localStream: MediaStream | null; remoteStreams: RemoteStream[]; peers: CallPeer[]; currentUserID: string; participantNames: Record<string, string>; canModerate: boolean; error: string; isConnecting: boolean; isConnected: boolean; isMuted: boolean; isCameraOff: boolean; isSharingScreen: boolean; onShare: (options: ScreenShareOptions) => Promise<void>; onStopShare: () => void; onToggleMute: () => void; onToggleCamera: () => Promise<void>; onModerate: (userID: string, action: 'mute' | 'unmute' | 'remove') => Promise<void>; onEnd: () => Promise<void> }
export function CallOverlay({ localStream, remoteStreams, peers, currentUserID, participantNames, canModerate, error, isConnecting, isConnected, isMuted, isCameraOff, isSharingScreen, onShare, onStopShare, onToggleMute, onToggleCamera, onModerate, onEnd }: Props) {
  const [focusedStreamID, setFocusedStreamID] = useState<string | null>(null)
  const [shareModalOpen, setShareModalOpen] = useState(false)
  const otherPeers = peers.filter(peer => peer.user_id !== currentUserID)
  const title = isConnected ? 'Соединение установлено' : isConnecting ? 'Подключаемся…' : 'Соединение потеряно'
  // The local tile's own kind is known outright (isSharingScreen), never guessed
  // — unlike a remote peer, there is no SFU roster entry for our own stream to
  // read a label from.
  const local = localStream ? [{ id: 'local', stream: localStream, sourceStreamID: isSharingScreen ? 'local-screen' : undefined }] : []
  const localTracks = isSharingScreen ? [{ track_id: 'local-screen', stream_id: 'local-screen', kind: 'video', label: 'screen' }] : undefined
  const nameFor = (userID: string, fallback: string) => participantNames[userID] ?? fallback
  const sections = new Map<string, StreamEntry[]>()
  for (const stream of remoteStreams) {
    const owner = stream.ownerID ?? otherPeers.find(peer => peer.tracks?.some(track => track.stream_id === stream.sourceStreamID))?.user_id
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
  return <section className="call-overlay" aria-label="Звонок">
    <header><div><span className="call-label">Звонок</span><h2>{title}</h2></div><Button variant="danger" onClick={() => void onEnd()}>Завершить</Button></header>
    {error ? <Notice>{error}</Notice> : null}
    {remoteStreams.filter(entry => entry.stream.getAudioTracks().length > 0).map(entry => <AudioSink key={entry.id} stream={entry.stream} />)}
    {focused
      ? <div className="call-focused"><div className="call-focused__main"><VideoTile stream={focused.stream} muted={isLocal(focused.id)} label={labelFor(focused.id)} focused onFocus={() => toggleFocus(focused.id)} /></div><div className="call-focused__strip">{allStreams.filter(stream => stream.id !== focused.id && stream.stream.getVideoTracks().length > 0).map(stream => <VideoTile key={stream.id} stream={stream.stream} muted={isLocal(stream.id)} label={labelFor(stream.id)} focused={false} onFocus={() => toggleFocus(stream.id)} />)}</div></div>
      : <div className="call-grid"><ParticipantSection title={nameFor(currentUserID, 'Вы')} streams={local} tracks={localTracks} muted focusedID={focusedStreamID} onFocus={toggleFocus} />{[...sections].map(([userID, streams], index) => <ParticipantSection key={userID} title={nameFor(userID, `Собеседник ${index + 1}`)} streams={streams} tracks={peers.find(peer => peer.user_id === userID)?.tracks} focusedID={focusedStreamID} onFocus={toggleFocus} />)}{isConnecting ? <div className="call-waiting"><Spinner /><span>Настраиваем защищённое соединение</span></div> : null}{isConnected && otherPeers.length === 0 ? <div className="call-waiting">Ожидаем собеседника</div> : null}</div>}
    <footer>
      <Button variant="secondary" onClick={onToggleMute}>{isMuted ? 'Включить микрофон' : 'Выключить микрофон'}</Button>
      <Button variant="secondary" onClick={() => void onToggleCamera()}>{isCameraOff ? 'Включить камеру' : 'Выключить камеру'}</Button>
      <Button variant="secondary" onClick={() => isSharingScreen ? onStopShare() : setShareModalOpen(true)}>{isSharingScreen ? 'Остановить демонстрацию' : 'Показать экран'}</Button>
      {canModerate ? otherPeers.map(peer => <div className="call-peer" key={peer.user_id}><span>{nameFor(peer.user_id, 'Участник')} · {peer.muted ? 'без звука' : 'со звуком'}</span><Button variant="ghost" onClick={() => void onModerate(peer.user_id, peer.muted ? 'unmute' : 'mute')}>{peer.muted ? 'Включить звук' : 'Выключить звук'}</Button><Button variant="danger" onClick={() => void onModerate(peer.user_id, 'remove')}>Исключить</Button></div>) : null}
    </footer>
    {shareModalOpen ? <ShareScreenModal onCancel={() => setShareModalOpen(false)} onConfirm={options => { setShareModalOpen(false); void onShare(options) }} /> : null}
  </section>
}

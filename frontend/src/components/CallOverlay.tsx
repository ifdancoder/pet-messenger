import { useEffect, useRef, useState } from 'react'
import type { MouseEvent, MouseEventHandler } from 'react'
import type { CallPeer, RemoteStream, ScreenShareOptions } from '../hooks/useCall'
import { screenFrameRates, screenResolutions } from '../hooks/useCall'
import { Button, Notice, Spinner } from './ui'

type StreamEntry = Pick<RemoteStream, 'id' | 'stream' | 'sourceStreamID' | 'ownerID'>
type TileProps = { stream: MediaStream; audioStream?: MediaStream; muted?: boolean; label: string; focused: boolean; onFocus: () => void; onContextMenu?: MouseEventHandler }
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
// sharedAudioContext is reused everywhere: browsers cap how many
// AudioContexts a page may create, but one context happily hosts any number
// of analyser nodes.
let sharedAudioContext: AudioContext | null = null
function getAudioContext() {
  if (!sharedAudioContext) sharedAudioContext = new AudioContext()
  if (sharedAudioContext.state === 'suspended') void sharedAudioContext.resume()
  return sharedAudioContext
}
// Drives a CSS custom property straight through a ref, never React state:
// this ticks every animation frame, and running that through useState would
// re-render the whole tile 60 times a second for a purely cosmetic effect.
// The ref attaches directly to whatever element should carry the reactive
// outline — the avatar's own mark, or a video tile's whole frame — so the
// same hook drives both the mic indicator and a screen share's independent
// one without a separate overlay element for either.
function useAudioLevelRef<T extends HTMLElement>(stream?: MediaStream) {
  const target = useRef<T>(null)
  useEffect(() => {
    if (!stream?.getAudioTracks().length) return
    let analyser: AnalyserNode
    let source: MediaStreamAudioSourceNode
    try {
      const ctx = getAudioContext()
      source = ctx.createMediaStreamSource(stream)
      analyser = ctx.createAnalyser()
      analyser.fftSize = 256
      analyser.smoothingTimeConstant = 0.6
      source.connect(analyser)
    } catch {
      return
    }
    const data = new Uint8Array(analyser.frequencyBinCount)
    let raf = 0
    const tick = () => {
      analyser.getByteFrequencyData(data)
      let sum = 0
      for (const value of data) sum += value
      const level = Math.min(1, (sum / data.length / 255) * 2.5)
      target.current?.style.setProperty('--level', level.toFixed(3))
      raf = requestAnimationFrame(tick)
    }
    tick()
    return () => { cancelAnimationFrame(raf); source.disconnect(); analyser.disconnect() }
  }, [stream])
  return target
}
function VideoTile({ stream, audioStream, muted, label, focused, onFocus, onContextMenu }: TileProps) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  // audioStream is only ever this specific tile's own audio (a screen
  // share's tab/system sound) — a camera tile gets none, so its outline
  // just never lights up, no separate code path needed.
  const frame = useAudioLevelRef<HTMLElement>(audioStream)
  return <figure className="call-video" ref={frame} onContextMenu={onContextMenu}><video ref={video} autoPlay playsInline muted={muted} /><figcaption>{label}</figcaption><button className="call-video__focus" type="button" aria-label={focused ? 'Свернуть видео' : 'Развернуть видео'} title={focused ? 'Свернуть' : 'Развернуть'} onClick={onFocus}><FocusIcon focused={focused} /></button></figure>
}
// size drives the CSS var the mark reads its dimensions from — one avatar
// implementation for both the full placeholder tile and the small identity
// chip that stays put once a video tile takes the placeholder's place.
function Avatar({ label, stream, size, onContextMenu }: { label: string; stream?: MediaStream; size: 'lg' | 'sm'; onContextMenu?: MouseEventHandler }) {
  const name = label.split(' · ').at(-1) || label
  const mark = useAudioLevelRef<HTMLSpanElement>(stream)
  return <span className={`call-avatar call-avatar--${size}`} onContextMenu={onContextMenu}><span className="call-avatar__mark" ref={mark} aria-hidden="true">{name.slice(0, 1).toUpperCase()}</span></span>
}
function EmptyVideoTile({ label, stream, onContextMenu }: { label: string; stream?: MediaStream; onContextMenu?: MouseEventHandler }) {
  return <figure className="call-video call-video--placeholder"><div className="call-video__profile"><Avatar label={label} stream={stream} size="lg" onContextMenu={onContextMenu} /><figcaption>{label}</figcaption></div></figure>
}
function ParticipantSection({ title, streams, tracks, muted, focusedID, onFocus, onVolumeMenu }: { title: string; streams: StreamEntry[]; tracks?: CallPeer['tracks']; muted?: boolean; focusedID: string | null; onFocus: (id: string) => void; onVolumeMenu?: (event: MouseEvent, kind: 'mic' | 'screen') => void }) {
  const videoStreams = streams.filter(entry => entry.stream.getVideoTracks().length > 0).sort((left, right) => Number(isScreenShare(left, tracks)) - Number(isScreenShare(right, tracks)))
  // Two distinct audio sources can coexist for one participant — their mic,
  // and their screen's own tab/system audio — so "the audio entry" is never
  // unambiguous; each one drives its own outline independently.
  const micStream = streams.find(entry => entry.stream.getAudioTracks().length > 0 && !isScreenShare(entry, tracks))?.stream
  const screenAudioStream = streams.find(entry => entry.stream.getAudioTracks().length > 0 && isScreenShare(entry, tracks))?.stream
  const mediaStatus = videoStreams.length > 1 ? 'видео и демонстрация' : videoStreams.length === 1 ? 'медиа' : 'камера выключена'
  const micMenu = onVolumeMenu && ((event: MouseEvent) => onVolumeMenu(event, 'mic'))
  // The identity chip lives in the header, not in the media area, precisely
  // so it survives whatever the media area is showing: a placeholder is
  // never mounted alongside a video tile (only one or the other), but the
  // mic it controls keeps playing through AudioSink regardless — without a
  // permanent spot for it, starting a screen share (or the camera) took the
  // only place that offered it clean off the page.
  return <article className="call-participant"><header><span className="call-participant__identity"><Avatar label={title} stream={micStream} size="sm" onContextMenu={micMenu} /><span>{title}</span></span><small>{mediaStatus}</small></header><div className="call-participant__media">{videoStreams.length ? videoStreams.map(entry => { const screen = isScreenShare(entry, tracks); return <VideoTile key={entry.id} stream={entry.stream} audioStream={screen ? screenAudioStream : undefined} muted={muted} label={screen ? 'Демонстрация экрана' : 'Камера'} focused={focusedID === entry.id} onFocus={() => onFocus(entry.id)} onContextMenu={onVolumeMenu && (event => onVolumeMenu(event, screen ? 'screen' : 'mic'))} /> }) : <EmptyVideoTile label={title} stream={micStream} onContextMenu={micMenu} />}</div></article>
}
// A remote participant's mic (and a shared screen's own tab/system audio)
// arrives as its own audio-only MediaStream, never bundled into a video
// tile's srcObject — video tiles come and go with the grid/focused layout,
// but audio must keep playing regardless of which tiles happen to be
// mounted right now. Rendered off-screen, once per audio track, always.
function AudioSink({ stream, volume }: { stream: MediaStream; volume: number }) {
  const audio = useRef<HTMLAudioElement>(null)
  useEffect(() => { if (audio.current) audio.current.srcObject = stream }, [stream])
  useEffect(() => { if (audio.current) audio.current.volume = volume }, [volume])
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
type VolumeMenuState = { x: number; y: number; key: string; label: string }
// A right-click volume control only ever changes local playback (this
// AudioSink's own <audio>.volume) — never anything the SFU or the other
// participant sees, same as any other call app's per-person volume slider.
function VolumeMenu({ menu, value, onChange, onClose }: { menu: VolumeMenuState; value: number; onChange: (volume: number) => void; onClose: () => void }) {
  const left = Math.min(menu.x, window.innerWidth - 216)
  const top = Math.min(menu.y, window.innerHeight - 108)
  return <div className="volume-menu-backdrop" onClick={onClose} onContextMenu={event => { event.preventDefault(); onClose() }}>
    <div className="volume-menu" style={{ left, top }} role="menu" aria-label={menu.label} onClick={event => event.stopPropagation()}>
      <span className="volume-menu__label">{menu.label}</span>
      <input type="range" min={0} max={100} value={Math.round(value * 100)} onChange={event => onChange(Number(event.target.value) / 100)} />
      <span className="volume-menu__value">{Math.round(value * 100)}%</span>
    </div>
  </div>
}

type Props = { localStream: MediaStream | null; localScreenStream: MediaStream | null; remoteStreams: RemoteStream[]; peers: CallPeer[]; currentUserID: string; participantNames: Record<string, string>; canModerate: boolean; error: string; isConnecting: boolean; isConnected: boolean; isMuted: boolean; isCameraOff: boolean; isSharingScreen: boolean; reconnectExhausted: boolean; onReconnect: () => void; onShare: (options: ScreenShareOptions) => Promise<void>; onStopShare: () => void; onToggleMute: () => void; onToggleCamera: () => Promise<void>; onModerate: (userID: string, action: 'mute' | 'unmute' | 'remove') => Promise<void>; onEnd: () => Promise<void> }
export function CallOverlay({ localStream, localScreenStream, remoteStreams, peers, currentUserID, participantNames, canModerate, error, isConnecting, isConnected, isMuted, isCameraOff, isSharingScreen, reconnectExhausted, onReconnect, onShare, onStopShare, onToggleMute, onToggleCamera, onModerate, onEnd }: Props) {
  const [focusedStreamID, setFocusedStreamID] = useState<string | null>(null)
  const [shareModalOpen, setShareModalOpen] = useState(false)
  // Keyed "<userID>:mic" / "<userID>:screen", not by stream id: a stream id
  // is only as stable as the RTCPeerConnection that reported it, so a
  // reconnect would silently reset every volume choice back to 100% if this
  // keyed on that instead.
  const [volumes, setVolumes] = useState<Record<string, number>>({})
  const [volumeMenu, setVolumeMenu] = useState<VolumeMenuState | null>(null)
  const openVolumeMenu = (event: MouseEvent, key: string, label: string) => {
    event.preventDefault()
    setVolumeMenu({ x: event.clientX, y: event.clientY, key, label })
  }
  const otherPeers = peers.filter(peer => peer.user_id !== currentUserID)
  const title = isConnected ? 'Соединение установлено' : isConnecting ? 'Подключаемся…' : reconnectExhausted ? 'Не удалось переподключиться' : 'Соединение потеряно'
  // Camera and screen are two independent slots here, exactly like a remote
  // participant's own separate published tracks — never one shared "current
  // local stream" toggled by whichever of shareScreen/toggleCamera last ran.
  // That used to let turning the camera on or off while already sharing the
  // screen silently swap out what the self-view was displaying without
  // updating what it was labeled, or collapse it to an empty placeholder.
  // The local tile's own kind is known outright, never guessed — unlike a
  // remote peer, there is no SFU roster entry for our own streams to read a
  // label from, so localTracks declares "screen" unconditionally and only
  // the local-screen entry's sourceStreamID ever actually matches it.
  const local: StreamEntry[] = []
  if (localStream) local.push({ id: 'local-camera', stream: localStream, sourceStreamID: undefined, ownerID: undefined })
  if (localScreenStream) local.push({ id: 'local-screen', stream: localScreenStream, sourceStreamID: 'local-screen', ownerID: undefined })
  const localTracks = [{ track_id: 'local-screen', stream_id: 'local-screen', kind: 'video', label: 'screen' }]
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
    if (isLocal(id)) return nameFor(currentUserID, 'Вы')
    const owner = [...sections.entries()].find(([, streams]) => streams.some(stream => stream.id === id))?.[0]
    return owner ? nameFor(owner, 'Собеседник') : 'Собеседник'
  }
  const isLocal = (id: string) => id === 'local-camera' || id === 'local-screen'
  // Shared by the focused main tile and its filmstrip: a remote entry's
  // owner and kind (mic vs. this specific participant's screen audio) are
  // resolved the same way ParticipantSection resolves them for the grid.
  const contextMenuFor = (entry: StreamEntry): MouseEventHandler | undefined => {
    if (!entry.ownerID) return undefined
    const owner = entry.ownerID
    const tracks = peers.find(peer => peer.user_id === owner)?.tracks
    const screen = isScreenShare(entry, tracks)
    const key = `${owner}:${screen ? 'screen' : 'mic'}`
    const label = `${screen ? 'Демонстрация' : 'Микрофон'} — ${labelFor(entry.id)}`
    return event => openVolumeMenu(event, key, label)
  }
  return <section className="call-overlay" aria-label="Звонок">
    <header><div><span className="call-label">Звонок</span><h2>{title}</h2></div><Button variant="danger" onClick={() => void onEnd()}>Завершить</Button></header>
    {error ? <Notice>{error}</Notice> : null}
    {remoteStreams.filter(entry => entry.stream.getAudioTracks().length > 0).map(entry => {
      const tracks = peers.find(peer => peer.user_id === entry.ownerID)?.tracks
      const key = entry.ownerID ? `${entry.ownerID}:${isScreenShare(entry, tracks) ? 'screen' : 'mic'}` : entry.id
      return <AudioSink key={entry.id} stream={entry.stream} volume={volumes[key] ?? 1} />
    })}
    {reconnectExhausted
      ? <div className="call-waiting call-waiting--lost"><span>Автоматически переподключиться не удалось</span><Button onClick={onReconnect}>Переподключиться</Button></div>
      : focused
      ? <div className="call-focused"><div className="call-focused__main"><VideoTile stream={focused.stream} muted={isLocal(focused.id)} label={labelFor(focused.id)} focused onFocus={() => toggleFocus(focused.id)} onContextMenu={contextMenuFor(focused)} /></div><div className="call-focused__strip">{allStreams.filter(stream => stream.id !== focused.id && stream.stream.getVideoTracks().length > 0).map(stream => <VideoTile key={stream.id} stream={stream.stream} muted={isLocal(stream.id)} label={labelFor(stream.id)} focused={false} onFocus={() => toggleFocus(stream.id)} onContextMenu={contextMenuFor(stream)} />)}</div></div>
      : <div className="call-grid"><ParticipantSection title={nameFor(currentUserID, 'Вы')} streams={local} tracks={localTracks} muted focusedID={focusedStreamID} onFocus={toggleFocus} />{[...sections].map(([userID, streams], index) => <ParticipantSection key={userID} title={nameFor(userID, `Собеседник ${index + 1}`)} streams={streams} tracks={peers.find(peer => peer.user_id === userID)?.tracks} focusedID={focusedStreamID} onFocus={toggleFocus} onVolumeMenu={(event, kind) => openVolumeMenu(event, `${userID}:${kind}`, `${kind === 'screen' ? 'Демонстрация' : 'Микрофон'} — ${nameFor(userID, `Собеседник ${index + 1}`)}`)} />)}{isConnecting ? <div className="call-waiting"><Spinner /><span>Настраиваем защищённое соединение</span></div> : null}{isConnected && otherPeers.length === 0 ? <div className="call-waiting">Ожидаем собеседника</div> : null}</div>}
    <footer>
      <Button variant="secondary" onClick={onToggleMute}>{isMuted ? 'Включить микрофон' : 'Выключить микрофон'}</Button>
      <Button variant="secondary" onClick={() => void onToggleCamera()}>{isCameraOff ? 'Включить камеру' : 'Выключить камеру'}</Button>
      <Button variant="secondary" onClick={() => isSharingScreen ? onStopShare() : setShareModalOpen(true)}>{isSharingScreen ? 'Остановить демонстрацию' : 'Показать экран'}</Button>
      {canModerate ? otherPeers.map(peer => <div className="call-peer" key={peer.user_id}><span>{nameFor(peer.user_id, 'Участник')} · {peer.muted ? 'без звука' : 'со звуком'}</span><Button variant="ghost" onClick={() => void onModerate(peer.user_id, peer.muted ? 'unmute' : 'mute')}>{peer.muted ? 'Включить звук' : 'Выключить звук'}</Button><Button variant="danger" onClick={() => void onModerate(peer.user_id, 'remove')}>Исключить</Button></div>) : null}
    </footer>
    {shareModalOpen ? <ShareScreenModal onCancel={() => setShareModalOpen(false)} onConfirm={options => { setShareModalOpen(false); void onShare(options) }} /> : null}
    {volumeMenu ? <VolumeMenu menu={volumeMenu} value={volumes[volumeMenu.key] ?? 1} onChange={volume => setVolumes(previous => ({ ...previous, [volumeMenu.key]: volume }))} onClose={() => setVolumeMenu(null)} /> : null}
  </section>
}

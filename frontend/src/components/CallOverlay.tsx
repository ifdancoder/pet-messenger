import { useEffect, useRef, useState } from 'react'
import type { MouseEvent, MouseEventHandler, PointerEvent as ReactPointerEvent } from 'react'
import type { CallPeer, RemoteStream, ScreenShareOptions } from '../hooks/useCall'
import { entryMatchesTrack, screenFrameRates, screenResolutions } from '../hooks/useCall'
import { Button, IconButton, Notice, Spinner } from './ui'

type StreamEntry = Pick<RemoteStream, 'id' | 'stream' | 'sourceStreamID' | 'mid' | 'ownerID'>
type TileProps = { stream: MediaStream; audioStream?: MediaStream; muted?: boolean; label: string; focused: boolean; onFocus: () => void; onContextMenu?: MouseEventHandler }
// entryMatchesTrack checks mid first (this connection's own
// RTCRtpTransceiver.mid, matched against the roster's per-viewer
// TrackInfo.mid) and only falls back to the MediaStream id the track arrived
// on when one side lacks a mid — see entryMatchesTrack's own doc comment.
function isScreenShare(entry: StreamEntry, tracks?: CallPeer['tracks']) {
  return tracks?.find(track => entryMatchesTrack(entry, track))?.label === 'screen'
}
// A participant's mic is never ambiguous even when their screen's own
// tab/system audio is also present as a second audio-only stream — this is
// the one shared rule both the grid header and the focused view's identity
// chip resolve it by, so they never drift into showing two different mic
// streams for the same person.
function findMicStream(streams: StreamEntry[], tracks?: CallPeer['tracks']) {
  return streams.find(entry => entry.stream.getAudioTracks().length > 0 && !isScreenShare(entry, tracks))?.stream
}
function FocusIcon({ focused }: { focused: boolean }) {
  return focused
    ? <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3v6H3M15 3v6h6M3 15h6v6M21 15h-6v6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
    : <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3H3v6M15 3h6v6M3 15v6h6M21 15v6h-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
function MinimizeIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 14h4v4M18 10h-4V6M14 14l7 7M10 10 3 3" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
// sharedAudioContext is reused everywhere: browsers cap how many
// AudioContexts a page may create, but one context happily hosts any number
// of analyser nodes.
let sharedAudioContext: AudioContext | null = null
export function getAudioContext() {
  if (!sharedAudioContext) sharedAudioContext = new AudioContext()
  if (sharedAudioContext.state === 'suspended') void sharedAudioContext.resume()
  return sharedAudioContext
}
// Drives a CSS custom property straight through a ref, never React state:
// this ticks every animation frame, and running that through useState would
// re-render the whole tile 60 times a second for a purely cosmetic effect.
// The ref attaches directly to whatever element should carry the reactive
// outline — the identity chip's own frame, or a video tile's whole frame —
// so the same hook drives both without a separate overlay element for either.
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
// IdentityChip is the one persistent "who is this" affordance in the whole
// overlay: an avatar mark plus name inside a single frame. The frame itself
// — not the avatar mark alone — carries both the speaking glow and the
// right-click target, so a viewer always has one obvious bordered thing to
// find and right-click for that person's volume, regardless of which tile
// layout (grid, focused, video, placeholder) currently surrounds it.
function IdentityChip({ label, stream, onContextMenu }: { label: string; stream?: MediaStream; onContextMenu?: MouseEventHandler }) {
  const name = label.split(' · ').at(-1) || label
  const frame = useAudioLevelRef<HTMLSpanElement>(stream)
  return <span className="identity-chip" ref={frame} onContextMenu={onContextMenu}><span className="identity-chip__mark" aria-hidden="true">{name.slice(0, 1).toUpperCase()}</span><span className="identity-chip__name">{label}</span></span>
}
function VideoTile({ stream, audioStream, muted, label, focused, onFocus, onContextMenu }: TileProps) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  // audioStream is only ever a screen share's own tab/system audio (see
  // ParticipantSection) — a camera tile gets none, so its outline just
  // never lights up, no separate code path needed. This is in addition to,
  // not instead of, the header's own "Демонстрация" chip: the video tile
  // itself is what a viewer is actually looking at, so the glow and
  // right-click volume control belong there too, not only in a small chip
  // easy to miss above it.
  const frame = useAudioLevelRef<HTMLElement>(audioStream)
  return <figure className="call-video" ref={frame} onContextMenu={onContextMenu}><video ref={video} autoPlay playsInline muted={muted} /><figcaption>{label}</figcaption><button className="call-video__focus" type="button" aria-label={focused ? 'Свернуть видео' : 'Развернуть видео'} title={focused ? 'Свернуть' : 'Развернуть'} onClick={onFocus}><FocusIcon focused={focused} /></button></figure>
}
function ParticipantSection({ title, streams, tracks, muted, focusedID, onFocus, onVolumeMenu }: { title: string; streams: StreamEntry[]; tracks?: CallPeer['tracks']; muted?: boolean; focusedID: string | null; onFocus: (id: string) => void; onVolumeMenu?: (event: MouseEvent, kind: 'mic' | 'screen') => void }) {
  const videoStreams = streams.filter(entry => entry.stream.getVideoTracks().length > 0).sort((left, right) => Number(isScreenShare(left, tracks)) - Number(isScreenShare(right, tracks)))
  // Two distinct audio sources can coexist for one participant — their mic,
  // and their screen's own tab/system audio — so "the audio entry" is never
  // unambiguous; each one drives its own outline independently.
  const micStream = findMicStream(streams, tracks)
  const screenAudioStream = streams.find(entry => entry.stream.getAudioTracks().length > 0 && isScreenShare(entry, tracks))?.stream
  const mediaStatus = videoStreams.length > 1 ? 'видео и демонстрация' : videoStreams.length === 1 ? 'медиа' : 'камера выключена'
  const micMenu = onVolumeMenu && ((event: MouseEvent) => onVolumeMenu(event, 'mic'))
  const screenMenu = onVolumeMenu && ((event: MouseEvent) => onVolumeMenu(event, 'screen'))
  // The header's identity chip (glow + right-click volume) is the one
  // permanent "who is this, how loud are they" affordance for this
  // participant — it never yields to a video tile, or a camera/screen
  // share turning on would silently take away the only way to adjust their
  // volume. A second chip for the screen's own tab/system audio appears
  // alongside it whenever that audio exists, exactly mirroring the mic
  // chip rather than relying on the video tile itself to carry a glow.
  return <article className="call-participant"><header><div className="call-participant__identities"><IdentityChip label={title} stream={micStream} onContextMenu={micMenu} />{screenAudioStream ? <IdentityChip label="Демонстрация" stream={screenAudioStream} onContextMenu={screenMenu} /> : null}</div><small>{mediaStatus}</small></header>{videoStreams.length ? <div className="call-participant__media">{videoStreams.map(entry => { const screen = isScreenShare(entry, tracks); return <VideoTile key={entry.id} stream={entry.stream} audioStream={screen ? screenAudioStream : undefined} muted={muted} label={screen ? 'Демонстрация экрана' : 'Камера'} focused={focusedID === entry.id} onFocus={() => onFocus(entry.id)} onContextMenu={screen ? screenMenu : undefined} /> })}</div> : null}</article>
}
// A remote participant's mic (and a shared screen's own tab/system audio)
// arrives as its own audio-only MediaStream, never bundled into a video
// tile's srcObject — video tiles come and go with the grid/focused layout,
// but audio must keep playing regardless of which tiles happen to be
// mounted right now, and regardless of whether the call is minimized: this
// is rendered unconditionally by CallOverlay, never inside the branch that
// switches between the full layout and the mini/PiP one.
function AudioSink({ stream, volume }: { stream: MediaStream; volume: number }) {
  const audio = useRef<HTMLAudioElement>(null)
  useEffect(() => { if (audio.current) audio.current.srcObject = stream }, [stream])
  useEffect(() => { if (audio.current) audio.current.volume = volume }, [volume])
  return <audio ref={audio} autoPlay hidden />
}
// PipVideoSink hosts the browser's own Picture-in-Picture window: a tiny,
// permanently-mounted, invisible <video> whose srcObject is whichever
// remote video is currently worth watching. It never renders on the page
// itself (real audio always comes from AudioSink, so this stays muted to
// avoid doubling it) — its only job is to be a valid PiP source so
// navigating to another chat can float the call's video above the rest of
// the app (and, with the OS, above the browser entirely) instead of just
// stopping being visible. requestPictureInPicture can refuse (no video
// track yet, a Firefox build without the API, no outstanding user gesture
// by the time this effect runs) — onExit is how the caller finds out its
// attempt didn't stick, so it can fall back to the compact bar instead of
// silently minimizing into nothing.
function PipVideoSink({ stream, active, onExit }: { stream: MediaStream | null; active: boolean; onExit: () => void }) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  useEffect(() => {
    const el = video.current
    if (!el) return
    if (active && stream && document.pictureInPictureEnabled) {
      if (document.pictureInPictureElement !== el) el.requestPictureInPicture().catch(() => onExit())
    } else if (!active && document.pictureInPictureElement === el) {
      void document.exitPictureInPicture().catch(() => {})
    }
  }, [active, stream, onExit])
  useEffect(() => {
    const el = video.current
    if (!el) return
    el.addEventListener('leavepictureinpicture', onExit)
    return () => el.removeEventListener('leavepictureinpicture', onExit)
  }, [onExit])
  return <video ref={video} autoPlay playsInline muted style={{ position: 'fixed', width: 1, height: 1, opacity: 0, pointerEvents: 'none', left: -9999 }} />
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
// elapsedLabel formats seconds as m:ss / h:mm:ss for the mini bar's call timer.
function elapsedLabel(totalSeconds: number) {
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${minutes}:${pad(seconds)}`
}
// MiniCallBar is what shows while minimized and no video is available to
// hand off to the browser's own Picture-in-Picture (an audio-only call, an
// unsupported browser, or PiP being refused this time) — a small persistent
// floating control strip rather than nothing, so leaving the call's chat
// never reads as leaving the call itself.
function MiniCallBar({ names, isMuted, onToggleMute, onEnd, onExpand, connectedSince }: { names: string[]; isMuted: boolean; onToggleMute: () => void; onEnd: () => void; onExpand: () => void; connectedSince: number | null }) {
  const [elapsed, setElapsed] = useState(() => connectedSince === null ? 0 : Math.max(0, Math.floor((Date.now() - connectedSince) / 1000)))
  useEffect(() => {
    if (connectedSince === null) return
    const id = window.setInterval(() => setElapsed(Math.max(0, Math.floor((Date.now() - connectedSince) / 1000))), 1000)
    return () => window.clearInterval(id)
  }, [connectedSince])
  // Dragging is layered on top of the plain click-to-expand behavior rather
  // than replacing it: a pointerdown that never moves past a small
  // threshold still expands the call on release, exactly like before —
  // only a real drag (mouse/touch/pen alike, via the Pointer Events API)
  // suppresses that and repositions the bar instead. Position is absolute
  // pixel coordinates once set, overriding the CSS default bottom-right
  // corner; clamped to the viewport so a drag can never push it off-screen.
  const [position, setPosition] = useState<{ x: number; y: number } | null>(null)
  const drag = useRef<{ startX: number; startY: number; originX: number; originY: number; moved: boolean } | null>(null)
  const onPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return
    const rect = event.currentTarget.getBoundingClientRect()
    drag.current = { startX: event.clientX, startY: event.clientY, originX: position?.x ?? rect.left, originY: position?.y ?? rect.top, moved: false }
    event.currentTarget.setPointerCapture(event.pointerId)
  }
  const onPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const state = drag.current
    if (!state) return
    const dx = event.clientX - state.startX
    const dy = event.clientY - state.startY
    if (!state.moved && Math.hypot(dx, dy) < 4) return
    state.moved = true
    const rect = event.currentTarget.getBoundingClientRect()
    setPosition({
      x: Math.min(Math.max(0, state.originX + dx), window.innerWidth - rect.width),
      y: Math.min(Math.max(0, state.originY + dy), window.innerHeight - rect.height),
    })
  }
  const onPointerUp = () => {
    const wasDrag = drag.current?.moved
    drag.current = null
    if (!wasDrag) onExpand()
  }
  return <div
    className="call-mini-bar"
    role="button" tabIndex={0}
    style={position ? { left: position.x, top: position.y, right: 'auto', bottom: 'auto' } : undefined}
    onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp}
    onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') onExpand() }}
  >
    <span className="call-mini-bar__names">{names.length ? names.join(', ') : 'Звонок'}</span>
    <span className="call-mini-bar__timer">{elapsedLabel(elapsed)}</span>
    <IconButton label={isMuted ? 'Включить микрофон' : 'Выключить микрофон'} onClick={event => { event.stopPropagation(); onToggleMute() }}>{isMuted ? '🔇' : '🎙️'}</IconButton>
    <IconButton label="Завершить звонок" className="call-mini-bar__end" onClick={event => { event.stopPropagation(); onEnd() }}>✕</IconButton>
  </div>
}

type Props = { localStream: MediaStream | null; localScreenStream: MediaStream | null; remoteStreams: RemoteStream[]; peers: CallPeer[]; currentUserID: string; participantNames: Record<string, string>; canModerate: boolean; error: string; isConnecting: boolean; isConnected: boolean; isMuted: boolean; isCameraOff: boolean; isSharingScreen: boolean; reconnectExhausted: boolean; minimized: boolean; onToggleMinimize: () => void; onRestore: () => void; onReconnect: () => void; onShare: (options: ScreenShareOptions) => Promise<void>; onStopShare: () => void; onToggleMute: () => void; onToggleCamera: () => Promise<void>; onModerate: (userID: string, action: 'mute' | 'unmute' | 'remove') => Promise<void>; onEnd: () => Promise<void> }
export function CallOverlay({ localStream, localScreenStream, remoteStreams, peers, currentUserID, participantNames, canModerate, error, isConnecting, isConnected, isMuted, isCameraOff, isSharingScreen, reconnectExhausted, minimized, onToggleMinimize, onRestore, onReconnect, onShare, onStopShare, onToggleMute, onToggleCamera, onModerate, onEnd }: Props) {
  const [focusedStreamID, setFocusedStreamID] = useState<string | null>(null)
  const [shareModalOpen, setShareModalOpen] = useState(false)
  // Tracks when the call actually connected, so the mini bar's timer doesn't
  // restart just because the overlay got minimized. Derived from isConnected
  // rather than passed down some other way because "the moment it first
  // became true" isn't a value either side already has lying around.
  const [connectedSince, setConnectedSince] = useState<number | null>(null)
  useEffect(() => {
    if (isConnected) setConnectedSince(current => current ?? Date.now())
    else if (!isConnecting) setConnectedSince(null)
  }, [isConnected, isConnecting])
  // PiP can refuse an individual attempt (no active gesture by the time the
  // effect ran, browser declined it); pipDismissed remembers that for the
  // rest of this minimized stretch so it falls back to the mini bar instead
  // of retrying every render, and clears the moment the call is next shown
  // in full so a later minimize gets a fresh attempt.
  const [pipDismissed, setPipDismissed] = useState(false)
  useEffect(() => { if (!minimized) setPipDismissed(false) }, [minimized])
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
  // ownerOf falls back to a roster lookup (by the same mid/stream_id
  // entryMatchesTrack uses) whenever a stream's owner couldn't be parsed
  // straight out of its forwarded stream id at ontrack time — the only way
  // that ever happens is a receiver that never resolves sourceStreamID at
  // all, exactly the gap mid-based matching exists to cover.
  const ownerOf = (entry: Pick<RemoteStream, 'ownerID' | 'mid' | 'sourceStreamID'>) =>
    entry.ownerID ?? otherPeers.find(peer => peer.tracks?.some(track => entryMatchesTrack(entry, track)))?.user_id
  const sections = new Map<string, StreamEntry[]>()
  for (const stream of remoteStreams) {
    const owner = ownerOf(stream)
    const key = owner ?? `unknown:${stream.id}`
    sections.set(key, [...(sections.get(key) ?? []), stream])
  }
  for (const peer of otherPeers) {
    if (!sections.has(peer.user_id)) sections.set(peer.user_id, [])
  }
  const allStreams = [...local, ...[...sections.values()].flat()]
  const focused = focusedStreamID ? allStreams.find(stream => stream.id === focusedStreamID) : undefined
  const toggleFocus = (id: string) => setFocusedStreamID(current => current === id ? null : id)
  const isLocal = (id: string) => id === 'local-camera' || id === 'local-screen'
  const labelFor = (id: string) => {
    if (isLocal(id)) return nameFor(currentUserID, 'Вы')
    const owner = [...sections.entries()].find(([, streams]) => streams.some(stream => stream.id === id))?.[0]
    return owner ? nameFor(owner, 'Собеседник') : 'Собеседник'
  }
  // Shared by the focused main tile and its filmstrip: a remote entry's
  // owner and kind (mic vs. this specific participant's screen audio) are
  // resolved the same way ParticipantSection resolves them for the grid.
  const contextMenuFor = (entry: StreamEntry): MouseEventHandler | undefined => {
    const owner = ownerOf(entry)
    if (!owner) return undefined
    const tracks = peers.find(peer => peer.user_id === owner)?.tracks
    const screen = isScreenShare(entry, tracks)
    const key = `${owner}:${screen ? 'screen' : 'mic'}`
    const label = `${screen ? 'Демонстрация' : 'Громкость'} — ${labelFor(entry.id)}`
    return event => openVolumeMenu(event, key, label)
  }
  // The PiP candidate: the first remote video going, screen share preferred
  // over camera (same priority ParticipantSection sorts by) — an audio-only
  // call (or every remote camera off) has none, so PiP is never attempted
  // and the mini bar is what minimizing shows instead.
  const remoteVideoStreams = [...sections.values()].flat().filter(entry => entry.stream.getVideoTracks().length > 0)
    .sort((left, right) => Number(isScreenShare(right, peers.find(peer => peer.user_id === ownerOf(right))?.tracks)) - Number(isScreenShare(left, peers.find(peer => peer.user_id === ownerOf(left))?.tracks)))
  const pipStream = remoteVideoStreams[0]?.stream ?? null
  const pipActive = minimized && !pipDismissed && pipStream !== null
  return <section className={`call-overlay${minimized ? ' call-overlay--minimized' : ''}`} aria-label="Звонок">
    {remoteStreams.filter(entry => entry.stream.getAudioTracks().length > 0).map(entry => {
      const owner = ownerOf(entry)
      const tracks = peers.find(peer => peer.user_id === owner)?.tracks
      const key = owner ? `${owner}:${isScreenShare(entry, tracks) ? 'screen' : 'mic'}` : entry.id
      return <AudioSink key={entry.id} stream={entry.stream} volume={volumes[key] ?? 1} />
    })}
    <PipVideoSink stream={pipStream} active={pipActive} onExit={() => setPipDismissed(true)} />
    {minimized
      ? <MiniCallBar names={otherPeers.map(peer => nameFor(peer.user_id, 'Собеседник'))} isMuted={isMuted} onToggleMute={onToggleMute} onEnd={() => void onEnd()} onExpand={onRestore} connectedSince={connectedSince} />
      : <>
        <header><div><span className="call-label">Звонок</span><h2>{title}</h2></div><div className="call-overlay__header-actions"><IconButton label="Свернуть в попап" onClick={onToggleMinimize}><MinimizeIcon /></IconButton><Button variant="danger" onClick={() => void onEnd()}>Завершить</Button></div></header>
        {error ? <Notice>{error}</Notice> : null}
        {reconnectExhausted
          ? <div className="call-waiting call-waiting--lost"><span>Автоматически переподключиться не удалось</span><Button onClick={onReconnect}>Переподключиться</Button></div>
          : focused
          ? <div className="call-focused"><div className="call-focused__main"><VideoTile stream={focused.stream} muted={isLocal(focused.id)} label={labelFor(focused.id)} focused onFocus={() => toggleFocus(focused.id)} onContextMenu={contextMenuFor(focused)} /></div><div className="call-focused__strip">{allStreams.filter(stream => stream.id !== focused.id && stream.stream.getVideoTracks().length > 0).map(stream => <VideoTile key={stream.id} stream={stream.stream} muted={isLocal(stream.id)} label={labelFor(stream.id)} focused={false} onFocus={() => toggleFocus(stream.id)} onContextMenu={contextMenuFor(stream)} />)}</div></div>
          : <div className="call-grid"><ParticipantSection title={nameFor(currentUserID, 'Вы')} streams={local} tracks={localTracks} muted focusedID={focusedStreamID} onFocus={toggleFocus} />{[...sections].map(([userID, streams], index) => <ParticipantSection key={userID} title={nameFor(userID, `Собеседник ${index + 1}`)} streams={streams} tracks={peers.find(peer => peer.user_id === userID)?.tracks} focusedID={focusedStreamID} onFocus={toggleFocus} onVolumeMenu={(event, kind) => openVolumeMenu(event, `${userID}:${kind}`, `${kind === 'screen' ? 'Демонстрация' : 'Громкость'} — ${nameFor(userID, `Собеседник ${index + 1}`)}`)} />)}{isConnecting ? <div className="call-waiting"><Spinner /><span>Настраиваем защищённое соединение</span></div> : null}{isConnected && otherPeers.length === 0 ? <div className="call-waiting">Ожидаем собеседника</div> : null}</div>}
        <footer>
          <Button variant="secondary" onClick={onToggleMute}>{isMuted ? 'Включить микрофон' : 'Выключить микрофон'}</Button>
          <Button variant="secondary" onClick={() => void onToggleCamera()}>{isCameraOff ? 'Включить камеру' : 'Выключить камеру'}</Button>
          <Button variant="secondary" onClick={() => isSharingScreen ? onStopShare() : setShareModalOpen(true)}>{isSharingScreen ? 'Остановить демонстрацию' : 'Показать экран'}</Button>
          {canModerate ? otherPeers.map(peer => <div className="call-peer" key={peer.user_id}><span>{nameFor(peer.user_id, 'Участник')} · {peer.muted ? 'без звука' : 'со звуком'}</span><Button variant="ghost" onClick={() => void onModerate(peer.user_id, peer.muted ? 'unmute' : 'mute')}>{peer.muted ? 'Включить звук' : 'Выключить звук'}</Button><Button variant="danger" onClick={() => void onModerate(peer.user_id, 'remove')}>Исключить</Button></div>) : null}
        </footer>
        {shareModalOpen ? <ShareScreenModal onCancel={() => setShareModalOpen(false)} onConfirm={options => { setShareModalOpen(false); void onShare(options) }} /> : null}
        {volumeMenu ? <VolumeMenu menu={volumeMenu} value={volumes[volumeMenu.key] ?? 1} onChange={volume => setVolumes(previous => ({ ...previous, [volumeMenu.key]: volume }))} onClose={() => setVolumeMenu(null)} /> : null}
      </>}
  </section>
}

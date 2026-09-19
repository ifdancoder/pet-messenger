import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal, flushSync } from 'react-dom'
import type { CSSProperties, MouseEvent, MouseEventHandler, PointerEvent as ReactPointerEvent } from 'react'
import type { CallPeer, RemoteStream, ScreenShareOptions } from '../hooks/useCall'
import { entryMatchesTrack, screenFrameRates, screenResolutions } from '../hooks/useCall'
import { Avatar } from './Avatar'
import { Button, IconButton, Notice, Spinner } from './ui'

type StreamEntry = Pick<RemoteStream, 'id' | 'stream' | 'sourceStreamID' | 'mid' | 'ownerID'>
// hasAudio is only ever passed for a screen-share tile — undefined (not
// false) for a camera tile, which has no "is this share's own audio
// included" concept to show at all.
type TileProps = { stream: MediaStream; audioStream?: MediaStream; hasAudio?: boolean; muted?: boolean; label: string; focused: boolean; onFocus: () => void; onContextMenu?: MouseEventHandler; onSpeakingChange?: (speaking: boolean) => void }
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
// ownerOf falls back to a roster lookup (by the same mid/stream_id
// entryMatchesTrack uses) whenever a stream's owner couldn't be parsed
// straight out of its forwarded stream id at ontrack time — the only way
// that ever happens is a receiver that never resolves sourceStreamID at all,
// exactly the gap mid-based matching exists to cover.
function ownerOf(entry: Pick<RemoteStream, 'ownerID' | 'mid' | 'sourceStreamID'>, peers: CallPeer[]) {
  return entry.ownerID ?? peers.find(peer => peer.tracks?.some(track => entryMatchesTrack(entry, track)))?.user_id
}
// A MediaStream can outlive both its RTP and the roster frame that removed
// its publisher.  ownerID is deliberately kept on the stream for matching,
// but it must never by itself make an old stream renderable: the current
// roster is the authority on who is still in the call.
function belongsToRoster(entry: Pick<RemoteStream, 'ownerID' | 'mid' | 'sourceStreamID'>, peers: CallPeer[]) {
  const owner = ownerOf(entry, peers)
  return owner !== undefined && peers.some(peer => peer.user_id === owner)
}
// streamVolumeKey is the one key both playback (CallAudio, always mounted)
// and the right-click volume menu (only ever opened from the full view) key
// a participant's volume choice by — computed once here so the two can never
// drift out of sync on it. Keyed "<userID>:mic" / "<userID>:screen", not by
// stream id: a stream id is only as stable as the RTCPeerConnection that
// reported it, so a reconnect would silently reset every volume choice back
// to 100% if this keyed on that instead.
function streamVolumeKey(entry: StreamEntry, peers: CallPeer[]): string {
  const owner = ownerOf(entry, peers)
  if (!owner) return entry.id
  return `${owner}:${isScreenShare(entry, peers.find(peer => peer.user_id === owner)?.tracks) ? 'screen' : 'mic'}`
}
// A layout icon (one big pane + a small overlapping one), not corner
// brackets: this toggle switches the grid for the main+filmstrip layout,
// which is a different action from MaximizeIcon's real fullscreen, and the
// two used near-identical bracket glyphs that were easy to mix up side by
// side.
function FocusIcon({ focused }: { focused: boolean }) {
  return focused
    ? <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" fill="none" stroke="currentColor" strokeWidth="2" /></svg>
    : <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" fill="none" stroke="currentColor" strokeWidth="2" /><rect x="12.5" y="12.5" width="7.5" height="7.5" rx="1.2" fill="currentColor" /></svg>
}
function MaximizeIcon({ active }: { active: boolean }) {
  return active
    ? <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 4v5H4M15 4v5h5M15 20v-5h5M9 20v-5H4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
    : <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 9V4h5M15 4h5v5M20 15v5h-5M9 20H4v-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
function MinimizeIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 14h4v4M18 10h-4V6M14 14l7 7M10 10 3 3" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
// SignalIcon carries connection state through shape (wifi arcs, full for a
// good connection; a "no signal" slash when lost) rather than color — it
// stays a plain white glyph in every state, like a phone's own signal
// indicator, with the actual wording still available as its title tooltip.
function SignalIcon({ state }: { state: 'connected' | 'connecting' | 'lost' }) {
  if (state === 'lost') {
    return <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M5 12.55a11 11 0 0 1 5.17-2.92M13.83 9.63A11 11 0 0 1 19 12.55" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M8.53 16.11a6 6 0 0 1 6.95 0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="12" cy="20" r="1" fill="currentColor" />
      <line x1="2" y1="2" x2="22" y2="22" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  }
  return <svg viewBox="0 0 24 24" aria-hidden="true">
    <path d="M5 12.55a11 11 0 0 1 14.08 0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    <path d="M1.42 9a16 16 0 0 1 21.16 0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    <path d="M8.53 16.11a6 6 0 0 1 6.95 0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    <circle cx="12" cy="20" r="1" fill="currentColor" />
  </svg>
}
function ExpandIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 3H3v5M16 3h5v5M3 16v5h5M21 16v5h-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
function PagerIcon({ direction }: { direction: 'prev' | 'next' }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><polyline points={direction === 'prev' ? '15 18 9 12 15 6' : '9 18 15 12 9 6'} fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
// A drawn X, matching every other control's stroke-based style (round caps,
// same weight) — a plain "✕" text glyph here read as a different, flatter
// icon language than the rest of the row right next to it.
function EndCallIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /><line x1="6" y1="6" x2="18" y2="18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /></svg>
}
// MicIcon doubles as the local mute button's glyph and the badge overlaid on
// a remote participant's identity tile when their mic is off — muted is a
// visually exceptional state worth crossing the icon out for, unlike a
// camera or screen share simply being off (their default, unremarkable
// state), which get their own affordance instead of a slash.
function MicIcon({ muted }: { muted: boolean }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true">
    <path d="M12 1a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    <path d="M19 10v1a7 7 0 0 1-14 0v-1" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    <line x1="12" y1="18" x2="12" y2="22" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    <line x1="8" y1="22" x2="16" y2="22" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    {muted ? <line x1="2" y1="2" x2="22" y2="22" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /> : null}
  </svg>
}
// Screen-share audio status — distinct from MicIcon since a share's own
// tab/system audio is a separate on/off concept from the presenter's mic.
function SpeakerIcon({ muted }: { muted: boolean }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true">
    <path d="M4 9v6h4l5 4V5L8 9H4z" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    {muted
      ? <line x1="2" y1="2" x2="22" y2="22" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      : <path d="M16.5 8.5a5 5 0 0 1 0 7" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />}
  </svg>
}
function CameraIcon({ off }: { off: boolean }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true">
    <path d="M23 7l-7 5 7 5V7z" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    <rect x="1" y="5" width="15" height="14" rx="2" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    {off ? <line x1="1" y1="1" x2="23" y2="23" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /> : null}
  </svg>
}
function AcceptCallIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7.2 3.5 4.7 4.8c-.8.4-1.2 1.3-.9 2.2 1.9 6.3 6.8 11.2 13.1 13.1.9.3 1.8-.1 2.2-.9l1.3-2.5c.3-.7.1-1.5-.5-1.9l-3-1.7a1.6 1.6 0 0 0-1.8.1l-1.4 1.1a12.8 12.8 0 0 1-4-4l1.1-1.4c.5-.5.5-1.3.1-1.8l-1.7-3a1.5 1.5 0 0 0-2-.6Z" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
}
// Screen share has no "off" slash: unlike mic/camera, off is its default,
// unremarkable state — being active is what deserves visual weight, given
// via the is-active class on its IconButton (see App.css) instead.
function ScreenShareIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true">
    <rect x="2" y="3" width="20" height="14" rx="2" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    <line x1="8" y1="21" x2="16" y2="21" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    <line x1="12" y1="17" x2="12" y2="21" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
  </svg>
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
// outline — the identity tile's own frame, or a video tile's whole frame —
// so the same hook drives both without a separate overlay element for either.
// onSpeakingChange is read through a ref, not a dependency: an inline
// arrow passed fresh every render would otherwise tear down and rebuild
// the AnalyserNode on every render instead of once per actual stream
// change, resetting the (already-working) --level glow along with it.
function useAudioLevelRef<T extends HTMLElement>(stream?: MediaStream, onSpeakingChange?: (speaking: boolean) => void) {
  const target = useRef<T>(null)
  const onSpeakingChangeRef = useRef(onSpeakingChange)
  onSpeakingChangeRef.current = onSpeakingChange
  useEffect(() => {
    if (!stream?.getAudioTracks().length) { onSpeakingChangeRef.current?.(false); return }
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
    // Hysteresis, not a raw threshold crossing: "speaking" latches on
    // instantly but only releases after speakingHoldMs of quiet, so a
    // participant's card doesn't flicker back and forth in the
    // speaking-priority order on every short pause between words.
    const speakingHoldMs = 500
    let lastAboveThreshold = 0
    let wasSpeaking = false
    const tick = () => {
      analyser.getByteFrequencyData(data)
      let sum = 0
      for (const value of data) sum += value
      const level = Math.min(1, (sum / data.length / 255) * 2.5)
      target.current?.style.setProperty('--level', level.toFixed(3))
      const now = performance.now()
      if (level > 0.12) lastAboveThreshold = now
      const speaking = now - lastAboveThreshold < speakingHoldMs
      if (speaking !== wasSpeaking) { wasSpeaking = speaking; onSpeakingChangeRef.current?.(speaking) }
      raf = requestAnimationFrame(tick)
    }
    tick()
    return () => { cancelAnimationFrame(raf); source.disconnect(); analyser.disconnect(); onSpeakingChangeRef.current?.(false) }
  }, [stream])
  return target
}
// IdentityTile is the one persistent "who is this, how loud are they"
// affordance for a participant — a full tile the same size and weight as a
// video tile (sharing its glow and its figcaption label styling), not a
// small chip easy to overlook. It never yields to a video tile: a
// camera/screen share turning on would otherwise take away the only place
// to adjust that participant's volume.
// IdentityTile is only ever used inside a card whose own header already
// names this participant (see ParticipantSection below) — no figcaption
// here, unlike VideoTile's, so a plain voice-only card doesn't say the same
// name twice in the little vertical space it has.
function IdentityTile({ label, avatarUrl, token, stream, muted, onContextMenu, onSpeakingChange }: { label: string; avatarUrl?: string; token: string; stream?: MediaStream; muted?: boolean; onContextMenu?: MouseEventHandler; onSpeakingChange?: (speaking: boolean) => void }) {
  const name = label.split(' · ').at(-1) || label
  const frame = useAudioLevelRef<HTMLElement>(stream, onSpeakingChange)
  return <figure className="call-video call-video--identity" ref={frame} onContextMenu={onContextMenu}>
    <Avatar subject={{ avatarUrl, displayName: name }} token={token} className="identity-tile__mark" />
    {muted ? <span className="identity-tile__muted" aria-label="Микрофон выключен"><MicIcon muted /></span> : null}
  </figure>
}
// IdentityChip is IdentityTile's compact counterpart for a participant who
// already has at least one video tile: a full tile-sized box just to hold
// a mic ring and a name would compete with that video for grid space for
// no benefit, so this shrinks to a pill (name + avatar) that still
// carries the same speaking glow and right-click volume menu.
function IdentityChip({ label, avatarUrl, token, stream, muted, onContextMenu, onSpeakingChange }: { label: string; avatarUrl?: string; token: string; stream?: MediaStream; muted?: boolean; onContextMenu?: MouseEventHandler; onSpeakingChange?: (speaking: boolean) => void }) {
  const frame = useAudioLevelRef<HTMLButtonElement>(stream, onSpeakingChange)
  return <button type="button" className="call-participant__chip" ref={frame} onContextMenu={onContextMenu}>
    <Avatar subject={{ avatarUrl, displayName: label }} token={token} className="call-participant__chip-avatar" />
    <span className="call-participant__chip-name">{label}</span>
    {muted ? <span className="call-participant__chip-muted" aria-label="Микрофон выключен"><MicIcon muted /></span> : null}
  </button>
}
function VideoTile({ stream, audioStream, hasAudio, muted, label, focused, onFocus, onContextMenu, onSpeakingChange }: TileProps) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  // audioStream is only ever a screen share's own tab/system audio (see
  // ParticipantSection) — a camera tile gets none, so its outline just
  // never lights up, no separate code path needed.
  const frame = useAudioLevelRef<HTMLElement>(audioStream, onSpeakingChange)
  // The tile's aspect ratio should match what's actually being captured (a
  // phone camera's 9:16, an ultrawide screen share's 21:9, ...) rather than
  // a fixed guess — read it straight off the video element once its real
  // dimensions are known, and again on 'resize' since a shared window can
  // change size mid-call. Kept as state (not just an imperative style write
  // on the inline figure) because the maximized lightbox below is a second,
  // separate figure that needs the same ratio and can't reach into this
  // one's ref for it.
  const [ratio, setRatio] = useState<number>()
  const syncRatio = () => {
    const el = video.current
    if (el?.videoWidth && el.videoHeight) setRatio(el.videoWidth / el.videoHeight)
  }
  const ratioStyle = ratio ? ({ '--video-ratio': ratio } as CSSProperties) : undefined
  // Maximize opens a *separate* fullscreen element portaled straight to
  // document.body, rather than fullscreening this tile's own figure in
  // place. That figure sits inside .call-participant, which the coverflow
  // grow/shrink effect (useCallGrid) keeps a live `transform: scale(...)`
  // on — fullscreening a descendant of a transformed ancestor is a known
  // cross-browser hazard (the element can render at the wrong size/position,
  // or just get corrupted at the CSS-cascade/paint level), and the resize
  // firing on enter/exit was also feeding useCallGrid's own update() bad
  // measurements, which then stuck around after exiting since nothing else
  // re-triggers a recompute — surfacing as the pager arrows and card
  // centering drifting wrong after a maximize/minimize round-trip. A portal
  // to document.body has no transformed ancestors and never touches the
  // grid's own DOM, so neither problem can happen; the lightbox gets its
  // own <video> bound to the same stream rather than moving this one, so
  // the grid tile keeps occupying its normal space the whole time.
  const [isMaximized, setIsMaximized] = useState(false)
  const lightbox = useRef<HTMLElement>(null)
  const lightboxVideo = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (isMaximized && lightboxVideo.current) lightboxVideo.current.srcObject = stream }, [isMaximized, stream])
  useEffect(() => {
    const onChange = () => { if (document.fullscreenElement !== lightbox.current) setIsMaximized(false) }
    document.addEventListener('fullscreenchange', onChange)
    return () => document.removeEventListener('fullscreenchange', onChange)
  }, [])
  // requestFullscreen() must run inside the same task as the click that
  // triggered it, or some browsers silently refuse it once that click's
  // transient "user activation" has lapsed — calling it from a useEffect,
  // which only runs after React finishes committing the state update that
  // mounted the portal, was exactly that kind of deferred call. flushSync
  // forces the portal to mount synchronously, right here in the click
  // handler, so requestFullscreen() is still requested inside the original
  // gesture in every browser instead of only the more lenient ones.
  const toggleMaximize = () => {
    if (isMaximized) {
      setIsMaximized(false)
      if (document.fullscreenElement === lightbox.current) void document.exitFullscreen()
      return
    }
    flushSync(() => setIsMaximized(true))
    if (lightbox.current) void lightbox.current.requestFullscreen().catch(() => setIsMaximized(false))
  }
  return <>
    <figure className="call-video" ref={frame} style={ratioStyle} onContextMenu={onContextMenu}><video ref={video} autoPlay playsInline muted={muted} onLoadedMetadata={syncRatio} onResize={syncRatio} /><figcaption>{label}</figcaption>{hasAudio !== undefined ? <span className={`call-video__audio${hasAudio ? '' : ' call-video__audio--off'}`} aria-label={hasAudio ? 'Звук демонстрации включён' : 'Звук демонстрации не подключён'} data-tooltip={hasAudio ? 'Со звуком' : 'Без звука'}><SpeakerIcon muted={!hasAudio} /></span> : null}<button className="call-video__maximize" type="button" aria-label={isMaximized ? 'Свернуть на весь экран' : 'На весь экран'} data-tooltip={isMaximized ? 'Свернуть' : 'На весь экран'} onClick={toggleMaximize}><MaximizeIcon active={isMaximized} /></button><button className="call-video__focus" type="button" aria-label={focused ? 'Свернуть видео' : 'Развернуть видео'} data-tooltip={focused ? 'Свернуть' : 'Развернуть'} onClick={onFocus}><FocusIcon focused={focused} /></button></figure>
    {isMaximized ? createPortal(<figure className="call-video call-video--lightbox" ref={lightbox} style={ratioStyle}><video ref={lightboxVideo} autoPlay playsInline muted={muted} /><figcaption>{label}</figcaption><button className="call-video__maximize" type="button" aria-label="Свернуть на весь экран" data-tooltip="Свернуть" onClick={toggleMaximize}><MaximizeIcon active /></button></figure>, document.body) : null}
  </>
}
// gridRef attaches the pager/reorder hook's per-card element ref straight
// to this card's own root — see useCallGrid below; onSpeakingChange is
// tagged by source ('mic' vs 'screen') since a participant with a screen
// share has two independent audio sources that can each make them count
// as "currently speaking" for the grid's speaking-first ordering.
function ParticipantSection({ title, avatarUrl, token, streams, tracks, muted, micMuted, ringing, showSpeaking = true, focusedID, onFocus, onVolumeMenu, gridRef, onSpeakingChange }: { title: string; avatarUrl?: string; token: string; streams: StreamEntry[]; tracks?: CallPeer['tracks']; muted?: boolean; micMuted?: boolean; ringing?: boolean; showSpeaking?: boolean; focusedID: string | null; onFocus: (id: string) => void; onVolumeMenu?: (event: MouseEvent, kind: 'mic' | 'screen') => void; gridRef?: (el: HTMLElement | null) => void; onSpeakingChange?: (source: 'mic' | 'screen', speaking: boolean) => void }) {
  const videoStreams = streams.filter(entry => entry.stream.getVideoTracks().length > 0).sort((left, right) => Number(isScreenShare(left, tracks)) - Number(isScreenShare(right, tracks)))
  // Two distinct audio sources can coexist for one participant — their mic,
  // and their screen's own tab/system audio — so "the audio entry" is never
  // unambiguous; each one drives its own outline independently.
  const micStream = findMicStream(streams, tracks)
  const screenAudioStream = streams.find(entry => entry.stream.getAudioTracks().length > 0 && isScreenShare(entry, tracks))?.stream
  const micMenu = onVolumeMenu && ((event: MouseEvent) => onVolumeMenu(event, 'mic'))
  const screenMenu = onVolumeMenu && ((event: MouseEvent) => onVolumeMenu(event, 'screen'))
  const onMicSpeaking = showSpeaking && onSpeakingChange ? (speaking: boolean) => onSpeakingChange('mic', speaking) : undefined
  const onScreenSpeaking = showSpeaking && onSpeakingChange ? (speaking: boolean) => onSpeakingChange('screen', speaking) : undefined
  // A participant with any video gets a compact identity chip instead of
  // the full-size IdentityTile — a whole tile just to hold a mic ring and
  // a name competed with their actual video for grid space, when a small
  // pill (matching the call-design-concept artifact) carries the same
  // glow and right-click volume menu just as well.
  const hasVideo = videoStreams.length > 0
  return <article className={`call-participant${ringing ? ' call-participant--ringing' : ''}`} ref={gridRef} data-media-count={hasVideo ? videoStreams.length : 0}>
    {hasVideo
      ? <IdentityChip label={title} avatarUrl={avatarUrl} token={token} stream={showSpeaking ? micStream : undefined} muted={micMuted} onContextMenu={micMenu} onSpeakingChange={onMicSpeaking} />
      : <header><span className="call-participant__identity-text">{title}</span></header>}
    <div className="call-participant__media">
      {hasVideo ? null : <IdentityTile label={title} avatarUrl={avatarUrl} token={token} stream={showSpeaking ? micStream : undefined} muted={micMuted} onContextMenu={micMenu} onSpeakingChange={onMicSpeaking} />}
      {videoStreams.map(entry => { const screen = isScreenShare(entry, tracks); return <VideoTile key={entry.id} stream={entry.stream} audioStream={screen ? screenAudioStream : undefined} hasAudio={screen ? Boolean(screenAudioStream) : undefined} muted={muted} label={screen ? 'Демонстрация экрана' : 'Камера'} focused={focusedID === entry.id} onFocus={() => onFocus(entry.id)} onContextMenu={screen ? screenMenu : undefined} onSpeakingChange={screen ? onScreenSpeaking : undefined} /> })}
    </div>
  </article>
}
// A remote participant's mic (and a shared screen's own tab/system audio)
// arrives as its own audio-only MediaStream, never bundled into a video
// tile's srcObject — video tiles come and go with the grid/focused layout,
// but audio must keep playing regardless of which tiles happen to be
// mounted right now, and regardless of which view (full/docked/floating) is
// currently showing: this is rendered unconditionally by CallAudio, never
// inside any of the three view-specific components.
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
// CallAudio is the one persistent, always-mounted piece of a call: every
// remote audio track keeps playing, and PiP keeps offering the current
// remote video, no matter which of the three views (full, docked, floating)
// is currently on screen — none of those three ever mount their own audio
// or PiP, so a switch between them can never interrupt either.
export function CallAudio({ remoteStreams, peers, volumes, floating }: { remoteStreams: RemoteStream[]; peers: CallPeer[]; volumes: Record<string, number>; floating: boolean }) {
  // PiP can refuse an individual attempt (no active gesture by the time the
  // effect ran, browser declined it); pipDismissed remembers that for the
  // rest of this floating stretch so it falls back to the mini bar instead
  // of retrying every render, and clears the moment the call next stops
  // floating so a later float gets a fresh attempt.
  const [pipDismissed, setPipDismissed] = useState(false)
  useEffect(() => { if (!floating) setPipDismissed(false) }, [floating])
  // The PiP candidate: the first remote video going, screen share preferred
  // over camera — an audio-only call (or every remote camera off) has none,
  // so PiP is never attempted.
  const rosterStreams = remoteStreams.filter(entry => belongsToRoster(entry, peers))
  const remoteVideoStreams = rosterStreams.filter(entry => entry.stream.getVideoTracks().length > 0)
    .sort((left, right) => Number(isScreenShare(right, peers.find(peer => peer.user_id === ownerOf(right, peers))?.tracks)) - Number(isScreenShare(left, peers.find(peer => peer.user_id === ownerOf(left, peers))?.tracks)))
  const pipStream = remoteVideoStreams[0]?.stream ?? null
  const pipActive = floating && !pipDismissed && pipStream !== null
  return <>
    {rosterStreams.filter(entry => entry.stream.getAudioTracks().length > 0).map(entry =>
      <AudioSink key={entry.id} stream={entry.stream} volume={volumes[streamVolumeKey(entry, peers)] ?? 1} />)}
    <PipVideoSink stream={pipStream} active={pipActive} onExit={() => setPipDismissed(true)} />
  </>
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
    {/* This checkbox only tells the browser audio is wanted; the browser's
        own share-source picker (opened right after "Начать демонстрацию",
        outside this app's control) decides whether it actually captures
        any — and it never does for a single window, on any platform. */}
    <p className="share-modal__hint">Чтобы звук передавался, в окне выбора источника выберите «Вкладка Chrome» и включите «Также поделиться звуком вкладки», либо «Весь экран» со звуком системы. Отдельное окно приложения передать звук не может.</p>
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
// elapsedLabel formats seconds as m:ss / h:mm:ss for a call timer.
function elapsedLabel(totalSeconds: number) {
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${minutes}:${pad(seconds)}`
}
// useElapsed drives a live m:ss/h:mm:ss timer off when the call actually
// connected — shared by the docked bar and the floating bar, the two places
// a call's duration is shown outside the full view's own header.
function useElapsed(connectedSince: number | null) {
  const [elapsed, setElapsed] = useState(() => connectedSince === null ? 0 : Math.max(0, Math.floor((Date.now() - connectedSince) / 1000)))
  useEffect(() => {
    if (connectedSince === null) return
    const id = window.setInterval(() => setElapsed(Math.max(0, Math.floor((Date.now() - connectedSince) / 1000))), 1000)
    return () => window.clearInterval(id)
  }, [connectedSince])
  return elapsed
}
// MiniCallBar is the floating popup: shown once the user navigates away
// from the call's own conversation entirely, so leaving that chat never
// reads as leaving the call itself. A plain click (no drag) navigates back
// to that conversation — docked, not full-screen, so returning to it
// doesn't sight-block whatever chat context led back there.
export function MiniCallBar({ names, isMuted, onToggleMute, onEnd, onExpand, connectedSince }: { names: string[]; isMuted: boolean; onToggleMute: () => void; onEnd: () => void; onExpand: () => void; connectedSince: number | null }) {
  const elapsed = useElapsed(connectedSince)
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
    <IconButton label={isMuted ? 'Включить микрофон' : 'Выключить микрофон'} onClick={event => { event.stopPropagation(); onToggleMute() }}><MicIcon muted={isMuted} /></IconButton>
    <IconButton label="Выйти из звонка" className="call-mini-bar__end" onClick={event => { event.stopPropagation(); onEnd() }}><EndCallIcon /></IconButton>
  </div>
}
// useCallGrid ports the call-design-concept artifact's coverflow pager and
// speaking-priority reorder onto real participant cards — proven out over
// many iterations there before landing here, not reinvented from scratch:
// paging steers continuously toward the live center gap every frame
// (scrollToIndex below) rather than committing to a fixed destination, so
// it can't overshoot and jerk-correct the way an upfront-computed target
// did in an earlier version of that artifact; a reorder (driven by
// orderedKeys, whose order the caller derives from speaking state) animates
// with a FLIP transform from each card's old position to its new one
// instead of letting the browser's own reconciliation snap it there.
function useCallGrid(orderedKeys: string[]) {
  // State, not a plain ref: the track div unmounts and remounts as a fresh
  // DOM node whenever the focused view opens or closes (see CallOverlay),
  // and the effects below need to actually rerun against that new node —
  // a plain ref's identity never changes, so an effect keyed on it (or on
  // anything else that isn't itself part of a render) would never notice
  // the swap and would keep listening to the now-detached old element.
  const [track, setTrack] = useState<HTMLDivElement | null>(null)
  // A caller asking to center on a specific key (see requestScrollToKey)
  // right when the track happens to still be null — e.g. returning from
  // the focused view, which unmounts and remounts this whole element —
  // can't just call scrollToKey immediately: the effect that requests it
  // lives in the caller's own component, keyed on its own state (not
  // track), so by the time that effect body actually runs it can be
  // holding a closure from before the ref callback's setTrack cascaded
  // through and never see the real element at all. Keeping the request as
  // state instead of an imperative call means the effect that resolves it
  // lives here, next to track, and reruns with a fresh closure whenever
  // track itself changes - not just whenever the caller's own trigger did.
  const [pendingScrollKey, setPendingScrollKey] = useState<string | null>(null)
  const leadSpacerRef = useRef<HTMLDivElement>(null)
  const trailSpacerRef = useRef<HTMLDivElement>(null)
  const cardEls = useRef(new Map<string, HTMLElement>())
  const prevRects = useRef(new Map<string, DOMRect>())
  const animRef = useRef(0)
  const [prevDisabled, setPrevDisabled] = useState(true)
  const [nextDisabled, setNextDisabled] = useState(true)
  const orderedKeysDep = JSON.stringify(orderedKeys)

  const registerCard = (key: string) => (el: HTMLElement | null) => {
    if (el) cardEls.current.set(key, el)
    else cardEls.current.delete(key)
  }

  // A card's own rendered width may already reflect however much this same
  // growth effect has scaled it up — reading flexBasis instead of the
  // current (possibly scaled) box avoids feeding that back into itself.
  const naturalWidth = (el: HTMLElement) => {
    const fromBasis = parseFloat(getComputedStyle(el).flexBasis)
    return Number.isFinite(fromBasis) ? fromBasis : el.getBoundingClientRect().width
  }

  const cards = () => orderedKeys.map(key => cardEls.current.get(key)).filter((el): el is HTMLElement => Boolean(el))

  const update = () => {
    if (!track) return
    const list = cards()
    const cs = getComputedStyle(track)
    const padLeft = parseFloat(cs.paddingLeft) || 0
    const padRight = parseFloat(cs.paddingRight) || 0
    const clientW = track.clientWidth
    if (list.length && leadSpacerRef.current && trailSpacerRef.current) {
      const firstW = naturalWidth(list[0])
      const lastW = naturalWidth(list[list.length - 1])
      leadSpacerRef.current.style.width = Math.max(0, clientW / 2 - padLeft - firstW / 2 - firstW * 0.135) + 'px'
      trailSpacerRef.current.style.width = Math.max(0, clientW / 2 - padRight - lastW / 2 - lastW * 0.135) + 'px'
    }
    // Disabled state is "is the edge card already centered", not "is
    // scrollLeft near the scrollable max" — the two disagree once the lead/
    // trail spacers (sized above so the first/last card CAN reach center)
    // are wider than a plain scrollLeft-vs-max comparison expects, which
    // left the last-card case needing a manual drag past where pageNext
    // could reach to make the arrow disappear.
    const max = track.scrollWidth - track.clientWidth
    if (!list.length || max <= 1) {
      setPrevDisabled(true)
      setNextDisabled(true)
    } else {
      const current = closestIndex()
      setPrevDisabled(current <= 0)
      setNextDisabled(current >= list.length - 1)
    }
    const trackRect = track.getBoundingClientRect()
    const centerX = trackRect.left + trackRect.width / 2
    const halfSpan = trackRect.width / 2 || 1
    list.forEach(card => {
      const rect = card.getBoundingClientRect()
      const cardCenter = rect.left + rect.width / 2
      const distance = Math.min(Math.abs(cardCenter - centerX) / halfSpan, 1)
      const cardW = naturalWidth(card)
      const growthPx = cardW * 0.27 * (1 - distance) - cardW * 0.155 * distance
      const scale = (1 + growthPx / rect.width).toFixed(4)
      card.dataset.scale = scale
      card.style.transform = `scale(${scale})`
      const halfGrowth = Math.max(growthPx, 0) / 2
      card.style.marginLeft = halfGrowth + 'px'
      card.style.marginRight = halfGrowth + 'px'
    })
  }

  const closestIndex = () => {
    if (!track) return 0
    const trackRect = track.getBoundingClientRect()
    const centerX = trackRect.left + trackRect.width / 2
    let best = 0
    let bestDist = Infinity
    cards().forEach((card, i) => {
      const r = card.getBoundingClientRect()
      const dist = Math.abs((r.left + r.right) / 2 - centerX)
      if (dist < bestDist) { bestDist = dist; best = i }
    })
    return best
  }

  const scrollToIndex = (index: number) => {
    const list = cards()
    if (!track || !list.length) return
    const card = list[Math.max(0, Math.min(list.length - 1, index))]
    const delta = () => {
      const trackRect = track.getBoundingClientRect()
      const centerX = trackRect.left + trackRect.width / 2
      const r = card.getBoundingClientRect()
      return (r.left + r.right) / 2 - centerX
    }
    const runId = ++animRef.current
    const start = performance.now()
    const step = (now: number) => {
      if (runId !== animRef.current) return
      const d = delta()
      if (Math.abs(d) < 0.4 || now - start > 900) { track.scrollLeft += d; return }
      track.scrollLeft += d * 0.22
      requestAnimationFrame(step)
    }
    requestAnimationFrame(step)
  }

  const pagePrev = () => scrollToIndex(closestIndex() - 1)
  const pageNext = () => scrollToIndex(closestIndex() + 1)
  // Re-centers on a specific participant by their orderedKeys entry
  // ('local' or a userID) — used when returning from the focused view so
  // the pager lands back on whoever was focused instead of wherever the
  // freshly remounted track happens to start.
  const scrollToKey = (key: string) => {
    const index = orderedKeys.indexOf(key)
    if (index >= 0) scrollToIndex(index)
  }
  // Resolves a pending requestScrollToKey once the track is actually
  // there. Deliberately keyed on `track` (not just pendingScrollKey): see
  // the state's own comment for why the caller's request can't just call
  // scrollToKey directly.
  useEffect(() => {
    if (!track || pendingScrollKey === null) return
    scrollToKey(pendingScrollKey)
    setPendingScrollKey(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [track, pendingScrollKey])

  // FLIP the reorder: measure where each card actually is right now (its
  // resting spot from the previous run of this effect), let this render's
  // new order + update() settle it into its new spot, then bridge the two
  // with an animated transform instead of the instant jump React's own
  // child-reordering would otherwise produce.
  useLayoutEffect(() => {
    const before = prevRects.current
    update()
    const after = new Map<string, DOMRect>()
    orderedKeys.forEach(key => {
      const el = cardEls.current.get(key)
      if (el) after.set(key, el.getBoundingClientRect())
    })
    orderedKeys.forEach(key => {
      const el = cardEls.current.get(key)
      const from = before.get(key)
      const to = after.get(key)
      if (!el || !from || !to) return
      const dx = (from.left + from.right) / 2 - (to.left + to.right) / 2
      const dy = (from.top + from.bottom) / 2 - (to.top + to.bottom) / 2
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) return
      const scale = el.dataset.scale ?? '1'
      el.style.transition = 'none'
      el.style.transform = `translate(${dx}px, ${dy}px) scale(${scale})`
      el.getBoundingClientRect() // force the line above to apply before the rAF below releases it
      requestAnimationFrame(() => {
        el.style.transition = 'transform .4s cubic-bezier(.22,.8,.25,1)'
        el.style.transform = `scale(${scale})`
        el.addEventListener('transitionend', () => { el.style.transition = '' }, { once: true })
      })
    })
    prevRects.current = after
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [track, orderedKeysDep])

  useEffect(() => {
    if (!track) return
    const onScroll = () => update()
    track.addEventListener('scroll', onScroll)
    // A ResizeObserver on the track itself, not a window 'resize' listener:
    // the track's own box can change size for reasons that never fire a
    // window resize at all (docking/undocking swaps --tile-w and the whole
    // layout; entering/exiting a video tile's own fullscreen repaints
    // everything behind it) — and even when the window does resize,
    // measuring on the track's own settled box is more reliable than
    // re-deriving it from a same-tick window event that can fire while
    // layout is still mid-transition.
    const observer = new ResizeObserver(() => update())
    observer.observe(track)
    return () => { track.removeEventListener('scroll', onScroll); observer.disconnect() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [track, orderedKeysDep])

  // Mouse drag-to-scroll via Pointer Events, matching the drag handling
  // MiniCallBar already uses elsewhere in this file, rather than a separate
  // mousedown/mousemove/mouseup trio. Keyed on `track`, not `[]`: the track
  // div unmounts and remounts as a fresh node across a focused-view
  // round-trip (see the state comment above), and a one-time effect would
  // keep dragging a now-detached element forever after that.
  useEffect(() => {
    if (!track) return
    let dragging = false
    let dragMoved = false
    let startX = 0
    let startScroll = 0
    const onDown = (event: PointerEvent) => {
      if (event.button !== 0) return
      // Capturing the pointer here also redirects the resulting click's
      // compatibility mouse event to the track itself (per the Pointer
      // Events spec), not whatever was actually under the cursor — so
      // starting a drag from inside a button silently ate every click on a
      // tile's own focus/maximize controls before it ever reached them.
      // Let a press that starts on a button behave like a normal click
      // instead of the start of a drag.
      if ((event.target as HTMLElement).closest('button')) return
      dragging = true
      dragMoved = false
      startX = event.clientX
      startScroll = track.scrollLeft
      track.setPointerCapture(event.pointerId)
    }
    const onMove = (event: PointerEvent) => {
      if (!dragging) return
      const dx = event.clientX - startX
      if (Math.abs(dx) > 4) dragMoved = true
      track.scrollLeft = startScroll - dx
    }
    const onUp = () => { dragging = false }
    // globalThis.MouseEvent, not the bare name: MouseEvent is shadowed at
    // this file's top by React's own synthetic event type, which a native
    // addEventListener('click', ...) below can't accept.
    const onClickCapture = (event: globalThis.MouseEvent) => { if (dragMoved) { event.stopPropagation(); event.preventDefault() } }
    track.addEventListener('pointerdown', onDown)
    track.addEventListener('pointermove', onMove)
    track.addEventListener('pointerup', onUp)
    track.addEventListener('click', onClickCapture, true)
    return () => {
      track.removeEventListener('pointerdown', onDown)
      track.removeEventListener('pointermove', onMove)
      track.removeEventListener('pointerup', onUp)
      track.removeEventListener('click', onClickCapture, true)
    }
  }, [track])

  return { trackRef: setTrack, leadSpacerRef, trailSpacerRef, registerCard, pagePrev, pageNext, prevDisabled, nextDisabled, requestScrollToKey: setPendingScrollKey }
}
type WaitingParticipant = { userID: string; name: string; avatarUrl?: string }
type IncomingActions = { onAcceptAudio: () => void; onAcceptCamera: () => void }
type Props = { localStream: MediaStream | null; localScreenStream: MediaStream | null; remoteStreams: RemoteStream[]; peers: CallPeer[]; currentUserID: string; token: string; participantNames: Record<string, string>; participantAvatars: Record<string, string>; canModerate: boolean; waitingParticipants: WaitingParticipant[]; incomingActions?: IncomingActions; showLocal?: boolean; showSpeaking?: boolean; error: string; isConnecting: boolean; isConnected: boolean; isMuted: boolean; isCameraOff: boolean; isSharingScreen: boolean; reconnectExhausted: boolean; connectedSince: number | null; volumes: Record<string, number>; onVolumeChange: (key: string, volume: number) => void; docked: boolean; onToggleDock: () => void; onReconnect: () => void; onShare: (options: ScreenShareOptions) => Promise<void>; onStopShare: () => void; onToggleMute: () => void; onToggleCamera: () => Promise<void>; onModerate: (userID: string, action: 'mute' | 'unmute' | 'remove') => Promise<void>; onEnd: () => Promise<void> }
// CallOverlay is the one call view for both full-screen and docked — docked
// is the exact same component and behavior, just laid out at its own
// smaller, dedicated scale (its own --tile-w and control sizes, see
// App.css) rather than a browser-zoomed copy of the full-screen layout,
// embedded above the call's own chat (see ChatPanel's callBanner slot)
// instead of covering the screen. CallAudio, mounted by the caller alongside
// whichever of the three views (this, docked or full; or the floating
// MiniCallBar) is current, is what keeps audio and PiP alive across
// switches between them — this component never touches either itself.
export function CallOverlay({ localStream, localScreenStream, remoteStreams, peers, currentUserID, token, participantNames, participantAvatars, canModerate, waitingParticipants, incomingActions, showLocal = true, showSpeaking = true, error, isConnecting, isConnected, isMuted, isCameraOff, isSharingScreen, reconnectExhausted, connectedSince, volumes, onVolumeChange, docked, onToggleDock, onReconnect, onShare, onStopShare, onToggleMute, onToggleCamera, onModerate, onEnd }: Props) {
  const elapsed = useElapsed(connectedSince)
  const [focusedStreamID, setFocusedStreamID] = useState<string | null>(null)
  const [shareModalOpen, setShareModalOpen] = useState(false)
  const [volumeMenu, setVolumeMenu] = useState<VolumeMenuState | null>(null)
  // Keyed "<userID>:mic" / "<userID>:screen" — a participant with a screen
  // share has two independent audio sources, either of which should count
  // toward them being "currently speaking" for ordering below.
  const [speaking, setSpeaking] = useState<Record<string, boolean>>({})
  const openVolumeMenu = (event: MouseEvent, key: string, label: string) => {
    event.preventDefault()
    setVolumeMenu({ x: event.clientX, y: event.clientY, key, label })
  }
  // A viewer is merely receiving an incoming docked preview. It is not in
  // the call yet, so it must neither get a participant tile nor suppress the
  // caller's ringing card for that person.
  const otherPeers = peers.filter(peer => peer.user_id !== currentUserID && peer.role !== 'viewer')
  // Do not let a stale browser MediaStream create a participant section.
  // The SFU's roster frame removes an exited participant before WebRTC has
  // necessarily fired `ended` for their old tracks.
  const liveRemoteStreams = remoteStreams.filter(entry => belongsToRoster(entry, otherPeers))
  const title = isConnected ? 'Соединение установлено' : isConnecting ? 'Подключаемся…' : reconnectExhausted ? 'Не удалось переподключиться' : 'Соединение потеряно'
  const statusClass = isConnected ? 'is-connected' : isConnecting ? 'is-connecting' : 'is-lost'
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
  const avatarFor = (userID: string) => participantAvatars[userID]
  const sections = new Map<string, StreamEntry[]>()
  for (const stream of liveRemoteStreams) {
    const owner = ownerOf(stream, otherPeers)
    const key = owner ?? `unknown:${stream.id}`
    sections.set(key, [...(sections.get(key) ?? []), stream])
  }
  for (const peer of otherPeers) {
    if (!sections.has(peer.user_id)) sections.set(peer.user_id, [])
  }
  const sectionKeys = [...sections.keys()]
  // "Собеседник N" fallback names are numbered by join order, not by
  // current on-screen position — otherwise a participant's own fallback
  // name would change every time the speaking-priority reorder below moves
  // them, which would read as a different person joining.
  const fallbackIndex = new Map(sectionKeys.map((key, i) => [key, i]))
  const isSpeakingKey = (key: string) => showSpeaking && (Boolean(speaking[`${key}:mic`]) || Boolean(speaking[`${key}:screen`]))
  // Speaking-first ordering, "Вы" excluded (rendered separately, always
  // first). A stable sort over sectionKeys' own join order (not a
  // separately persisted order) means two participants who are both quiet,
  // or both speaking, keep their existing relative order — only someone
  // actually starting or stopping speaking moves them past a neighbor.
  const orderedRemoteKeys = [...sectionKeys].sort((a, b) => Number(isSpeakingKey(b)) - Number(isSpeakingKey(a)))
  const waitingKeys = waitingParticipants.map(participant => `waiting:${participant.userID}`)
  const { trackRef, leadSpacerRef, trailSpacerRef, registerCard, pagePrev, pageNext, prevDisabled, nextDisabled, requestScrollToKey } = useCallGrid([...(showLocal ? ['local'] : []), ...orderedRemoteKeys, ...waitingKeys])
  const allStreams = [...local, ...[...sections.values()].flat()]
  const focused = focusedStreamID ? allStreams.find(stream => stream.id === focusedStreamID) : undefined
  const isLocal = (id: string) => id === 'local-camera' || id === 'local-screen'
  // The grid key ('local' or a userID) that owned the stream being
  // focused — used to re-center the pager on the right participant once
  // focus closes and the grid remounts. requestScrollToKey (not a direct
  // scrollToKey call here) matters: this closure's own `track` can be
  // stale by the time whoever reads it runs, since exiting focus remounts
  // the track as a fresh element - see its own comment in useCallGrid.
  const ownerKeyFor = (id: string) => isLocal(id) ? 'local' : [...sections.entries()].find(([, streams]) => streams.some(stream => stream.id === id))?.[0]
  const toggleFocus = (id: string) => {
    if (focusedStreamID === id) {
      const key = ownerKeyFor(id)
      if (key) requestScrollToKey(key)
      setFocusedStreamID(null)
    } else {
      setFocusedStreamID(id)
    }
  }
  const labelFor = (id: string) => {
    if (isLocal(id)) return nameFor(currentUserID, 'Вы')
    const owner = [...sections.entries()].find(([, streams]) => streams.some(stream => stream.id === id))?.[0]
    return owner ? nameFor(owner, 'Собеседник') : 'Собеседник'
  }
  // Shared by the focused main tile and its filmstrip: a remote entry's
  // owner and kind (mic vs. this specific participant's screen audio) are
  // resolved the same way ParticipantSection resolves them for the grid.
  const contextMenuFor = (entry: StreamEntry): MouseEventHandler | undefined => {
    const owner = ownerOf(entry, otherPeers)
    if (!owner) return undefined
    const tracks = peers.find(peer => peer.user_id === owner)?.tracks
    const screen = isScreenShare(entry, tracks)
    const key = `${owner}:${screen ? 'screen' : 'mic'}`
    const label = `${screen ? 'Демонстрация' : 'Громкость'} — ${labelFor(entry.id)}`
    return event => openVolumeMenu(event, key, label)
  }
  // Same "is this entry a screen share, and does that share carry its own
  // tab/system audio" resolution ParticipantSection does for the grid, but
  // for a focused-view entry (which may be the local screen, with no SFU
  // roster tracks to read a kind from at all).
  const isScreenEntry = (entry: StreamEntry): boolean => {
    if (entry.id === 'local-screen') return true
    if (entry.id === 'local-camera') return false
    const owner = ownerOf(entry, otherPeers)
    return isScreenShare(entry, owner ? peers.find(peer => peer.user_id === owner)?.tracks : undefined)
  }
  const screenAudioStreamFor = (entry: StreamEntry): MediaStream | undefined => {
    if (entry.id === 'local-screen') return localScreenStream && localScreenStream.getAudioTracks().length ? localScreenStream : undefined
    const owner = ownerOf(entry, otherPeers)
    if (!owner) return undefined
    const ownerTracks = peers.find(peer => peer.user_id === owner)?.tracks
    return (sections.get(owner) ?? []).find(s => s.stream.getAudioTracks().length > 0 && isScreenShare(s, ownerTracks))?.stream
  }
  return <section className={`call-overlay${docked ? ' call-overlay--docked' : ''}`} aria-label="Звонок">
    <header><div className="call-overlay__heading"><span className="call-label">Звонок</span>{isConnecting ? <span className="call-overlay__connection-note"><Spinner />Подключаемся…</span> : null}</div><div className="call-overlay__header-actions"><IconButton label={docked ? 'Развернуть звонок' : 'Свернуть звонок'} onClick={onToggleDock}>{docked ? <ExpandIcon /> : <MinimizeIcon />}</IconButton></div></header>
    {error ? <Notice>{error}</Notice> : null}
    {reconnectExhausted
      ? <div className="call-waiting call-waiting--lost"><span>Автоматически переподключиться не удалось</span><Button onClick={onReconnect}>Переподключиться</Button></div>
      : focused
      ? <div className="call-focused"><div className="call-focused__main"><VideoTile stream={focused.stream} audioStream={isScreenEntry(focused) ? screenAudioStreamFor(focused) : undefined} hasAudio={isScreenEntry(focused) ? Boolean(screenAudioStreamFor(focused)) : undefined} muted={isLocal(focused.id)} label={labelFor(focused.id)} focused onFocus={() => toggleFocus(focused.id)} onContextMenu={contextMenuFor(focused)} /></div><div className="call-focused__strip">{allStreams.filter(stream => stream.id !== focused.id && stream.stream.getVideoTracks().length > 0).map(stream => <VideoTile key={stream.id} stream={stream.stream} audioStream={isScreenEntry(stream) ? screenAudioStreamFor(stream) : undefined} hasAudio={isScreenEntry(stream) ? Boolean(screenAudioStreamFor(stream)) : undefined} muted={isLocal(stream.id)} label={labelFor(stream.id)} focused={false} onFocus={() => toggleFocus(stream.id)} onContextMenu={contextMenuFor(stream)} />)}</div></div>
      : <div className="call-tile-wrap">
          <button type="button" className="call-pager-btn call-pager-prev" disabled={prevDisabled} onClick={pagePrev} aria-label="Предыдущие участники"><PagerIcon direction="prev" /></button>
          <div className="call-tile-track" ref={trackRef}>
            <div className="call-tile-spacer" ref={leadSpacerRef} />
            {showLocal ? <ParticipantSection gridRef={registerCard('local')} title="Вы" avatarUrl={avatarFor(currentUserID)} token={token} streams={local} tracks={localTracks} muted micMuted={isMuted} showSpeaking={showSpeaking} focusedID={focusedStreamID} onFocus={toggleFocus} /> : null}
            {orderedRemoteKeys.map(userID => {
              const streams = sections.get(userID) ?? []
              const label = nameFor(userID, `Собеседник ${(fallbackIndex.get(userID) ?? 0) + 1}`)
              const peer = peers.find(p => p.user_id === userID)
              return <ParticipantSection
                key={userID}
                gridRef={registerCard(userID)}
                title={label}
                avatarUrl={avatarFor(userID)}
                token={token}
                streams={streams}
                tracks={peer?.tracks}
                micMuted={Boolean(peer?.muted) || Boolean(peer?.self_muted)}
                showSpeaking={showSpeaking}
                focusedID={focusedStreamID}
                onFocus={toggleFocus}
                onVolumeMenu={(event, kind) => openVolumeMenu(event, `${userID}:${kind}`, `${kind === 'screen' ? 'Демонстрация' : 'Громкость'} — ${label}`)}
                onSpeakingChange={(source, isSpeakingNow) => setSpeaking(prev => {
                  const flagKey = `${userID}:${source}`
                  if (Boolean(prev[flagKey]) === isSpeakingNow) return prev
                  return { ...prev, [flagKey]: isSpeakingNow }
                })}
              />
            })}
            {waitingParticipants.map(participant => <ParticipantSection
              key={`waiting:${participant.userID}`}
              gridRef={registerCard(`waiting:${participant.userID}`)}
              title={participant.name}
              avatarUrl={participant.avatarUrl}
              token={token}
              streams={[]}
              ringing
              focusedID={focusedStreamID}
              onFocus={toggleFocus}
            />)}
            <div className="call-tile-spacer" ref={trailSpacerRef} />
          </div>
          <button type="button" className="call-pager-btn call-pager-next" disabled={nextDisabled} onClick={pageNext} aria-label="Следующие участники"><PagerIcon direction="next" /></button>
        </div>}
    <footer>
      {/* The status icon lives here, not floating over the whole overlay,
          so it lines up on the same vertical center as the icon row next
          to it — color plus a native title tooltip carries the wording a
          header title used to (see statusClass above). Grouped with the
          call timer right next to it, matching the concept's own
          dock-status: icon and elapsed time read as one unit, sitting just
          to the left of the actual controls rather than floating alone. */}
      <span className="call-overlay__status-group">
        <span className={`call-overlay__status call-overlay__status--${statusClass}`} data-tooltip={title}><SignalIcon state={statusClass === 'is-connected' ? 'connected' : statusClass === 'is-connecting' ? 'connecting' : 'lost'} /></span>
        {connectedSince !== null ? <span className="call-overlay__timer">{elapsedLabel(elapsed)}</span> : null}
      </span>
      <div className="call-overlay__controls">
        {incomingActions
          ? <><button type="button" className="call-preview__accept" aria-label="Принять аудиозвонок" data-tooltip="Принять аудиозвонок" onClick={incomingActions.onAcceptAudio}><AcceptCallIcon /></button><button type="button" className="call-preview__accept" aria-label="Принять с камерой" data-tooltip="Принять с камерой" onClick={incomingActions.onAcceptCamera}><CameraIcon off={false} /></button></>
          : <><IconButton label={isMuted ? 'Включить микрофон' : 'Выключить микрофон'} onClick={onToggleMute}><MicIcon muted={isMuted} /></IconButton><IconButton label={isCameraOff ? 'Включить камеру' : 'Выключить камеру'} onClick={() => void onToggleCamera()}><CameraIcon off={isCameraOff} /></IconButton><IconButton label={isSharingScreen ? 'Остановить демонстрацию' : 'Показать экран'} className={isSharingScreen ? 'is-active' : ''} onClick={() => isSharingScreen ? onStopShare() : setShareModalOpen(true)}><ScreenShareIcon /></IconButton><IconButton label="Выйти из звонка" className="call-overlay__end" onClick={() => void onEnd()}><EndCallIcon /></IconButton></>}
      </div>
      {canModerate ? otherPeers.map(peer => <div className="call-peer" key={peer.user_id}><span>{nameFor(peer.user_id, 'Участник')} · {peer.muted ? 'без звука' : 'со звуком'}</span><Button variant="ghost" onClick={() => void onModerate(peer.user_id, peer.muted ? 'unmute' : 'mute')}>{peer.muted ? 'Включить звук' : 'Выключить звук'}</Button><Button variant="danger" onClick={() => void onModerate(peer.user_id, 'remove')}>Исключить</Button></div>) : null}
    </footer>
    {/* Portalled straight to document.body: both are position:fixed, and
        the docked view sits inside call-docked-wrapper's own
        overflow:hidden clip (see App.css) — a fixed descendant left inside
        that subtree could still get clipped by it in some browsers.
        Escaping to document.body sidesteps that, so these always land
        where the pointer actually was regardless of which view (full or
        docked) opened them. */}
    {shareModalOpen ? createPortal(<ShareScreenModal onCancel={() => setShareModalOpen(false)} onConfirm={options => { setShareModalOpen(false); void onShare(options) }} />, document.body) : null}
    {volumeMenu ? createPortal(<VolumeMenu menu={volumeMenu} value={volumes[volumeMenu.key] ?? 1} onChange={volume => onVolumeChange(volumeMenu.key, volume)} onClose={() => setVolumeMenu(null)} />, document.body) : null}
  </section>
}

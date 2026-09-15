import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { getAudioContext } from '../components/CallOverlay'
import { loadDevicePrefs, saveDevicePrefs } from '../lib/devicePrefs'
import { applyNoiseSuppression } from '../lib/noiseSuppressor'
import type { Conversation, Session } from '../types'

type Signal = { type: string; sdp?: string; candidate?: RTCIceCandidateInit; message?: string; peers?: CallPeer[] }
// mid is this connection's own RTCRtpTransceiver.mid for the track — the
// primary way to find this entry's kind/label in a peers frame. sourceStreamID
// (event.streams[0]?.id) is kept only as a fallback for a receiver that
// doesn't populate RTCTrackEvent.mid: mid is universally central to WebRTC's
// own offer/answer negotiation, but streams[0] is a legacy convenience some
// implementations may not populate the same way another one does — a
// mismatch there silently breaks a match that never had to depend on it.
export type RemoteStream = { id: string; stream: MediaStream; trackIDs: string[]; sourceStreamID?: string; mid?: string; kind?: string; ownerID?: string; firstSeenAt: number }

// staleStreamGraceMs guards against a benign race: the peers roster and the
// RTP that fires ontrack travel over separate transports (WS vs DTLS-SRTP)
// with no ordering guarantee between them, so a stream that is genuinely
// brand new can briefly be missing from the very first peers frame after it
// was created. Only prune an entry the roster no longer mentions once it has
// existed longer than this.
const staleStreamGraceMs = 1_500
export type CallPeer = { user_id: string; role: string; muted: boolean; tracks?: { track_id: string; stream_id: string; kind: string; label: string; mid?: string }[] }

// maxReconnectDelayMs caps the signalling socket's reconnect backoff, mirroring
// useRealtimeChat's message socket — the one place a dropped connection here
// already recovers on its own.
const maxReconnectDelayMs = 15_000
// reconnectTimeoutMs bounds how long automatic retries keep going after a
// drop: retrying forever left a call silently unrecoverable if whatever
// caused the drop wasn't transient (a peer with only one other participant
// leaving mid-blip, say) — the other side moves on with a working call while
// this one is stuck redialling a room that may not even exist any more.
// Past this, the app stops trying on its own and hands the decision back —
// a manual retry starts a fresh attempt window rather than resuming this one.
const reconnectTimeoutMs = 4 * 60_000

export type ScreenResolution = { label: string; width: number; height: number }

// Presets, not a free-form input: getDisplayMedia constraints are only ever
// "ideal" hints the browser and OS capture pipeline may ignore, so exposing
// exact numbers would promise a precision this can't deliver. Resolution and
// frame rate are picked independently — either can be pushed for a slide
// deck (4K, low motion) or a game (lower resolution, high fps) alike.
export const screenResolutions: ScreenResolution[] = [
  { label: '720p', width: 1280, height: 720 },
  { label: '1080p', width: 1920, height: 1080 },
  { label: '2K · 1440p', width: 2560, height: 1440 },
  { label: '4K · 2160p', width: 3840, height: 2160 },
]
export const screenFrameRates = [15, 24, 30, 60]

export type ScreenShareOptions = { width: number; height: number; frameRate: number; audio: boolean }
export const defaultScreenShareOptions: ScreenShareOptions = { width: 1920, height: 1080, frameRate: 30, audio: true }

// noiseSuppressionSetupTimeoutMs bounds how long localAudio waits for RNNoise
// (a WASM binary fetch + an AudioWorklet module registration, entirely out
// of this app's control once handed to the browser) before giving up and
// publishing the raw mic track instead. Noise suppression is an enhancement;
// it must never be able to block or delay the call itself connecting at all
// — a slow or wedged WASM/worklet load previously left start() hanging
// indefinitely on this await, so the whole call never even reached
// connectSignaling.
const noiseSuppressionSetupTimeoutMs = 3_000

function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T | null> {
  return new Promise(resolve => {
    const timer = window.setTimeout(() => resolve(null), ms)
    promise.then(
      value => { window.clearTimeout(timer); resolve(value) },
      () => { window.clearTimeout(timer); resolve(null) },
    )
  })
}

// Camera starts off: only the microphone is captured up front. toggleCamera
// acquires and publishes a camera track on demand.
async function localAudio(deviceId?: string, noiseSuppression?: boolean, ctx?: AudioContext) {
  const devices = await navigator.mediaDevices.enumerateDevices()
  if (!devices.some(device => device.kind === 'audioinput')) return { stream: new MediaStream(), cleanup: () => {} }
  try {
    const raw = await navigator.mediaDevices.getUserMedia({
      audio: { deviceId: deviceId ? { exact: deviceId } : undefined, echoCancellation: true, noiseSuppression: !noiseSuppression },
    })
    if (!noiseSuppression || !ctx) return { stream: raw, cleanup: () => {} }
    const suppressed = await withTimeout(applyNoiseSuppression(raw, ctx), noiseSuppressionSetupTimeoutMs)
    return suppressed ?? { stream: raw, cleanup: () => {} }
  } catch {
    return { stream: new MediaStream(), cleanup: () => {} }
  }
}

function ownerIDFromForwardedStream(streamID: string) {
  const match = /^pm-([0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})-/i.exec(streamID)
  return match?.[1]
}

// entryMatchesTrack decides whether a roster track entry describes the same
// forwarded track as this stream entry. mid is checked first and, when both
// sides have one, decides it outright: mid is central to WebRTC's own
// offer/answer negotiation on every conformant implementation, so it can't
// suffer the same mismatch stream_id can if a receiver doesn't populate
// RTCTrackEvent.streams the same way another one does. stream_id remains the
// fallback for whichever side lacks a mid (older data, or a receiver that
// genuinely never resolves one).
export function entryMatchesTrack(entry: { mid?: string; sourceStreamID?: string }, track: { mid?: string; stream_id: string }): boolean {
  if (entry.mid && track.mid) return entry.mid === track.mid
  return Boolean(entry.sourceStreamID) && entry.sourceStreamID === track.stream_id
}

// buildTrackKinds declares every currently-known track's kind for every
// transceiver the connection currently has — not just whichever ones the
// caller who triggered this particular offer happens to know about. Two
// operations that each add a track close together (turning the camera on
// right before sharing the screen, say) both mutate senders synchronously,
// outside the negotiation queue, so by the time either's queued offer
// actually goes out it may already carry a transceiver a *different* call
// added. Recomputing from the persistent knownKinds map on every single
// offer — instead of from only the kinds passed into this one call — means
// whichever offer wins the race still declares every mid correctly; the
// alternative silently let the first-out offer leave someone else's new
// track undeclared, which falls back to the SFU's stream-id guess and can
// never actually resolve to "screen".
function buildTrackKinds(connection: RTCPeerConnection, knownKinds: Map<MediaStreamTrack, string>): Record<string, string> | undefined {
  const trackKinds: Record<string, string> = {}
  for (const transceiver of connection.getTransceivers()) {
    const track = transceiver.sender.track
    const kind = track && knownKinds.get(track)
    if (kind && transceiver.mid) trackKinds[transceiver.mid] = kind
  }
  return Object.keys(trackKinds).length ? trackKinds : undefined
}

export function useCall(session: Session) {
  const peer = useRef<RTCPeerConnection | null>(null)
  const signal = useRef<WebSocket | null>(null)
  const local = useRef<MediaStream | null>(null)
  const screen = useRef<MediaStream | null>(null)
  const noiseCleanup = useRef<() => void>(() => {})
  const callID = useRef<string | null>(null)
  const sessionRef = useRef(session)
  useEffect(() => { sessionRef.current = session }, [session])
  const closedByClient = useRef(true)
  const reconnectAttempt = useRef(0)
  const reconnectTimer = useRef<number | undefined>(undefined)
  // Set on the first attempt of a drop, cleared on success (or a manual
  // retry) — scheduleReconnect gives up once Date.now() passes this, rather
  // than tracking a countdown that has to survive tab-throttling drift.
  const reconnectDeadline = useRef<number | null>(null)
  // scheduleReconnectRef breaks the mutual useCallback dependency between
  // connectSignaling (whose onclose handler needs to schedule a reconnect)
  // and scheduleReconnect (which needs to call connectSignaling again) —
  // always call scheduleReconnectRef.current(), never scheduleReconnect
  // directly, from inside connectSignaling.
  const scheduleReconnectRef = useRef<() => void>(() => {})
  // negotiationQueue serializes every SDP operation on peer.current: the
  // incoming offer/answer/candidate/peers handling AND every client-initiated
  // renegotiate() call (shareScreen, stopScreenShare, toggleCamera) alike.
  // createOffer/setLocalDescription on one RTCPeerConnection do not queue
  // themselves — two overlapping calls (e.g. turning the camera on right
  // before sharing the screen) can interleave, and a renegotiate() whose
  // track_kinds got built against a superseded offer sends a mid the server
  // never actually adopts. The video then falls back to the sfu's stream-id
  // guess, which can never actually detect "screen" — exactly the
  // mislabel-as-camera bug this queue exists to rule out.
  const negotiationQueue = useRef<Promise<unknown>>(Promise.resolve())
  const enqueueNegotiation = useCallback(<T,>(task: () => Promise<T>): Promise<T> => {
    const result = negotiationQueue.current.then(task, task)
    negotiationQueue.current = result.then(() => undefined, () => undefined)
    return result
  }, [])
  // knownKinds is a track's kind as a standing fact, not a one-off argument
  // to whichever renegotiate() call happened to publish it — see
  // buildTrackKinds. Persists across reconnects on purpose: a screen share
  // or camera track re-attached after a drop is still the same kind it
  // always was, with nothing new to declare.
  const knownKinds = useRef<Map<MediaStreamTrack, string>>(new Map())

  // Two independent slots, not one shared "whichever ran last" stream: camera
  // (localStream, always local.current — mic+camera, present the whole call)
  // and screen (localScreenStream, only while sharing) are published and
  // toggled by entirely separate actions. A single shared slot meant turning
  // the camera on or off *while already sharing the screen* clobbered
  // whichever the self-view was currently showing — the label (driven by
  // isSharingScreen) and the actual displayed content silently fell out of
  // sync, and stopping the camera while sharing collapsed the self-view to
  // an empty "camera off" placeholder even though the share was still live.
  const [localStream, setLocalStream] = useState<MediaStream | null>(null)
  const [localScreenStream, setLocalScreenStream] = useState<MediaStream | null>(null)
  const [remoteStreams, setRemoteStreams] = useState<RemoteStream[]>([])
  const [peers, setPeers] = useState<CallPeer[]>([])
  const [error, setError] = useState('')
  const [isConnecting, setIsConnecting] = useState(false)
  const [isOpen, setIsOpen] = useState(false)
  const [isMuted, setIsMuted] = useState(false)
  const [isCameraOff, setIsCameraOff] = useState(true)
  const [isSharingScreen, setIsSharingScreen] = useState(false)
  const [reconnectExhausted, setReconnectExhausted] = useState(false)

  const close = useCallback(async (end = false) => {
    closedByClient.current = true
    if (reconnectTimer.current) { window.clearTimeout(reconnectTimer.current); reconnectTimer.current = undefined }
    reconnectDeadline.current = null
    setReconnectExhausted(false)
    const id = callID.current
    signal.current?.close(); signal.current = null
    peer.current?.close(); peer.current = null
    screen.current?.getTracks().forEach(track => track.stop()); screen.current = null
    local.current?.getTracks().forEach(track => track.stop()); local.current = null
    noiseCleanup.current()
    knownKinds.current.clear()
    setLocalStream(null); setLocalScreenStream(null); setRemoteStreams([]); setPeers([]); setIsOpen(false); setIsConnecting(false); setIsMuted(false); setIsCameraOff(true); setIsSharingScreen(false)
    callID.current = null
    if (end && id) await api.endCall(id, sessionRef.current.access_token).catch(() => undefined)
  }, [])

  // connectSignaling opens the /rtc socket and drives one offer/answer cycle
  // over a fresh RTCPeerConnection. Used for the initial join and for every
  // reconnect: the SFU already treats a reconnect as a new session replacing
  // the old one (it never expects the same PeerConnection back), so a full
  // fresh negotiation is the supported path, not a special case.
  const connectSignaling = useCallback((join: { ticket: string; media_ws_url: string; ice_servers: RTCIceServer[] }) => {
    setError(''); setIsConnecting(true)
    // Every remote track this connection ever sees arrives fresh through its
    // own ontrack — old entries from a previous connection (a reconnect
    // replaces peer.current wholesale) are for a PeerConnection that no
    // longer exists. Nothing else would ever clear them: the peers-roster
    // pruning keys on stream_id + kind, and a track that's still genuinely
    // published looks identical whichever connection last reported it, so a
    // stale pre-reconnect entry and the fresh post-reconnect one for that
    // same mic both read as "live" and both survive — one more phantom
    // <audio> stacking its playback on every reconnect, forever.
    setRemoteStreams([])
    const connection = new RTCPeerConnection({ iceServers: join.ice_servers })
    peer.current = connection
    const pendingCandidates: RTCIceCandidateInit[] = []
    const enqueue = (task: () => Promise<void>) => {
      void enqueueNegotiation(task).catch(error => setError(error instanceof Error ? error.message : 'Ошибка соединения'))
    }

    connection.onconnectionstatechange = () => {
      switch (connection.connectionState) {
        case 'connected':
          reconnectAttempt.current = 0
          reconnectDeadline.current = null
          setReconnectExhausted(false)
          setIsConnecting(false)
          setIsOpen(true)
          break
        case 'failed':
        case 'closed':
          setIsConnecting(false)
          setIsOpen(false)
          break
      }
    }
    connection.onicecandidate = event => { if (event.candidate) signal.current?.send(JSON.stringify({ type: 'candidate', candidate: event.candidate.toJSON() })) }
    connection.ontrack = event => {
      // A browser may put camera and screen tracks into one remote MediaStream.
      // Keep a stream per track so each tile retains its own SFU label.
      const stream = new MediaStream([event.track])
      // event.track.id is a fresh id the browser mints on receipt — it never
      // matches whatever id the SFU put in the SDP, so it can't be used to
      // look up this track's kind. mid — this connection's own
      // RTCRtpTransceiver.mid — is the primary correlation key against the
      // peers roster's per-viewer TrackInfo.mid (see CallPeer): unlike the
      // MediaStream a track arrives on, mid is fundamental to every
      // conformant WebRTC implementation's own offer/answer negotiation, so
      // it can't go missing the way event.streams[0] can on a receiver that
      // doesn't populate it the way another one does. sourceStreamID is kept
      // only as that fallback.
      const mid = event.transceiver?.mid ?? undefined
      const sourceStreamID = event.streams[0]?.id
      setRemoteStreams(previous => {
        if (previous.some(item => item.id === event.track.id)) return previous
        return [...previous, { id: event.track.id, stream, trackIDs: [event.track.id], sourceStreamID, mid, kind: event.track.kind, ownerID: ownerIDFromForwardedStream(sourceStreamID ?? ''), firstSeenAt: Date.now() }]
      })
    }

    if (local.current) local.current.getTracks().forEach(track => connection.addTrack(track, local.current!))
    if (screen.current) {
      // Both tracks, not just video: a screen share's own tab/system audio
      // needs republishing after a reconnect exactly as much as its video
      // does — this used to only re-attach the video half, silently
      // dropping the audio half on every reconnect during an active share.
      screen.current.getTracks().forEach(track => connection.addTrack(track, screen.current!))
    }

    const socket = new WebSocket(`${join.media_ws_url}?ticket=${encodeURIComponent(join.ticket)}`)
    signal.current = socket
    socket.onopen = () => enqueue(async () => {
      const offer = await connection.createOffer()
      await connection.setLocalDescription(offer)
      socket.send(JSON.stringify({ type: 'offer', sdp: offer.sdp, track_kinds: buildTrackKinds(connection, knownKinds.current) }))
    })
    socket.onmessage = event => enqueue(async () => {
      const frame = JSON.parse(String(event.data)) as Signal
      if (frame.type === 'offer' && frame.sdp) {
        await connection.setRemoteDescription({ type: 'offer', sdp: frame.sdp })
        const answer = await connection.createAnswer(); await connection.setLocalDescription(answer)
        socket.send(JSON.stringify({ type: 'answer', sdp: answer.sdp }))
      } else if (frame.type === 'answer' && frame.sdp) await connection.setRemoteDescription({ type: 'answer', sdp: frame.sdp })
      else if (frame.type === 'candidate' && frame.candidate) {
        if (connection.remoteDescription) await connection.addIceCandidate(frame.candidate); else pendingCandidates.push(frame.candidate)
      } else if (frame.type === 'peers' && frame.peers) {
        setPeers(frame.peers)
        // The roster is authoritative for which streams are still actually
        // being forwarded — a track a participant stopped publishing (screen
        // share ended, camera turned off) drops out of it, but nothing else
        // ever prunes remoteStreams on its own: a MediaStream doesn't remove
        // itself just because its RTP stopped arriving. Without this, a
        // removed track's last-known state (including its label) stays
        // stuck on screen forever, all the way down to being relabeled
        // "Камера" once its own roster entry is gone.
        // entryMatchesTrack keys this on mid first, stream_id as its own
        // fallback — either way paired with kind: a camera video track and
        // its mic audio track can publish on the same MediaStream (same
        // stream_id) or, in principle, land back-to-back on a reused mid, so
        // neither alone can tell a dropped video track from a mic that is
        // still very much live on that same id/mid.
        const now = Date.now()
        setRemoteStreams(previous => previous.filter(entry => {
          if (!entry.mid && !entry.sourceStreamID) return true
          if (now - entry.firstSeenAt < staleStreamGraceMs) return true
          return frame.peers!.some(peer => peer.tracks?.some(track => track.kind === entry.kind && entryMatchesTrack(entry, track)))
        }))
      }
      else if (frame.type === 'removed') await close(false)
      else if (frame.type === 'error') setError(frame.message ?? 'Ошибка звонка')
      if (connection.remoteDescription) for (const candidate of pendingCandidates.splice(0)) await connection.addIceCandidate(candidate)
    })
    socket.onclose = event => {
      if (signal.current !== socket) return
      setIsOpen(false)
      setIsConnecting(false)
      if (closedByClient.current || event.code === 1000) return
      scheduleReconnectRef.current()
    }
  }, [close, enqueueNegotiation])

  // scheduleReconnect mints a fresh join ticket (the old one may have
  // expired, and api.joinCall transparently refreshes the access token if
  // that's what actually expired) and renegotiates from scratch. Retries
  // with capped backoff for up to reconnectTimeoutMs total — long enough to
  // ride out a background tab throttling timers or a brief network drop,
  // but not forever: past that, whatever's wrong is unlikely to be
  // transient (the room itself may be gone), and a silent retry loop the
  // user has no visibility into is worse than telling them and letting them
  // retry on their own terms.
  const scheduleReconnect = useCallback(() => {
    if (closedByClient.current || !callID.current) return
    if (reconnectDeadline.current === null) reconnectDeadline.current = Date.now() + reconnectTimeoutMs
    if (Date.now() >= reconnectDeadline.current) {
      setIsConnecting(false)
      setReconnectExhausted(true)
      return
    }
    const delay = Math.min(1_000 * 2 ** reconnectAttempt.current, maxReconnectDelayMs)
    reconnectAttempt.current += 1
    reconnectTimer.current = window.setTimeout(async () => {
      if (closedByClient.current || !callID.current) return
      try {
        const { join } = await api.joinCall(callID.current, sessionRef.current.access_token)
        if (closedByClient.current) return
        connectSignaling(join)
      } catch (error) {
        setError(error instanceof Error ? error.message : 'Не удалось переподключиться')
        scheduleReconnectRef.current()
      }
    }, delay)
  }, [connectSignaling])
  useEffect(() => { scheduleReconnectRef.current = scheduleReconnect }, [scheduleReconnect])

  // reconnectNow is the user's own escape hatch once automatic retries gave
  // up: same join+renegotiate path scheduleReconnect uses, just fired
  // immediately and starting a fresh reconnectTimeoutMs window rather than
  // resuming the exhausted one.
  const reconnectNow = useCallback(() => {
    if (!callID.current) return
    if (reconnectTimer.current) { window.clearTimeout(reconnectTimer.current); reconnectTimer.current = undefined }
    reconnectAttempt.current = 0
    reconnectDeadline.current = null
    setReconnectExhausted(false)
    setError('')
    setIsConnecting(true)
    void (async () => {
      try {
        const { join } = await api.joinCall(callID.current!, sessionRef.current.access_token)
        if (closedByClient.current) return
        connectSignaling(join)
      } catch (error) {
        setError(error instanceof Error ? error.message : 'Не удалось переподключиться')
        scheduleReconnectRef.current()
      }
    })()
  }, [connectSignaling])

  const start = useCallback(async (conversation: Conversation) => {
    closedByClient.current = false
    reconnectAttempt.current = 0
    reconnectDeadline.current = null
    setReconnectExhausted(false)
    setError(''); setIsConnecting(true)
    try {
      const { call, join } = await api.startCall(conversation.id, sessionRef.current.access_token)
      callID.current = call.id
      const prefs = loadDevicePrefs()
      const { stream: audio, cleanup } = await localAudio(prefs.audioInputId, prefs.noiseSuppression, prefs.noiseSuppression ? getAudioContext() : undefined)
      noiseCleanup.current = cleanup
      audio.getAudioTracks().forEach(track => knownKinds.current.set(track, 'mic'))
      local.current = audio; setLocalStream(audio)
      connectSignaling(join)
    } catch (error) { setError(error instanceof Error ? error.message : 'Не удалось начать звонок'); await close(false) }
  }, [close, connectSignaling])

  // renegotiate sends a fresh offer reflecting whatever the caller already
  // did to peer.current's senders (addTrack/removeTrack) — shareScreen,
  // stopScreenShare and toggleCamera all end with this instead of repeating
  // the offer/mid dance. It takes no kind arguments: a caller that just
  // published a track records its kind into knownKinds itself, synchronously,
  // in the same tick as the addTrack call that published it (see shareScreen,
  // toggleCamera) — recording it here instead, inside this queued task, would
  // reopen the exact race buildTrackKinds exists to close, since this task
  // might not actually run until well after some other caller's own addTrack
  // already put an undeclared transceiver on the connection.
  const renegotiate = useCallback(() => enqueueNegotiation(async () => {
    const connection = peer.current
    const socket = signal.current
    if (!connection || !socket || socket.readyState !== WebSocket.OPEN) return
    const offer = await connection.createOffer()
    await connection.setLocalDescription(offer)
    socket.send(JSON.stringify({ type: 'offer', sdp: offer.sdp, track_kinds: buildTrackKinds(connection, knownKinds.current) }))
  }), [enqueueNegotiation])

  // stopScreenShare tears down publishing, not just the local preview: a
  // disabled-but-still-attached sender would keep sending nothing to a
  // frozen last frame, which never falls back to the "no video" placeholder
  // for anyone else — only an actual removeTrack + renegotiation does.
  // Callable from the stop button and from the browser's own native "Stop
  // sharing" bar (track.onended) alike.
  const stopScreenShare = useCallback(() => {
    const display = screen.current
    if (!display) return
    const tracks = display.getTracks()
    for (const sender of peer.current?.getSenders() ?? []) {
      if (sender.track && tracks.includes(sender.track)) peer.current!.removeTrack(sender)
    }
    tracks.forEach(track => { track.stop(); knownKinds.current.delete(track) })
    screen.current = null
    setLocalScreenStream(null)
    setIsSharingScreen(false)
    void renegotiate()
  }, [renegotiate])

  const shareScreen = useCallback(async (options: ScreenShareOptions = defaultScreenShareOptions) => {
    if (!peer.current) return
    try {
      const display = await navigator.mediaDevices.getDisplayMedia({
        video: { width: { ideal: options.width }, height: { ideal: options.height }, frameRate: { ideal: options.frameRate } },
        audio: options.audio,
      })
      const track = display.getVideoTracks()[0]
      if (!track) return
      screen.current?.getTracks().forEach(previous => previous.stop())
      screen.current = display
      peer.current.addTrack(track, display)
      // Recorded in the same tick as addTrack, never inside the queued
      // renegotiate task that follows — see renegotiate's own comment for
      // why that ordering specifically is what keeps this race-free.
      knownKinds.current.set(track, 'screen')
      const audioTrack = display.getAudioTracks()[0]
      if (audioTrack) {
        peer.current.addTrack(audioTrack, display)
        // The tab/system audio track, when present, shares the screen's own
        // stream id (same source MediaStream), so it gets the same label.
        knownKinds.current.set(audioTrack, 'screen')
      }
      setLocalScreenStream(display)
      setIsSharingScreen(true)
      track.onended = () => { if (screen.current === display) stopScreenShare() }
      await renegotiate()
    } catch (error) { if (error instanceof Error && error.name !== 'NotAllowedError') setError(error.message) }
  }, [renegotiate, stopScreenShare])

  const toggleMute = useCallback(() => {
    const tracks = local.current?.getAudioTracks() ?? []
    if (!tracks.length) return
    const muted = tracks.some(track => track.enabled)
    tracks.forEach(track => { track.enabled = !muted })
    setIsMuted(muted)
  }, [])

  // Camera starts off (see localAudio) and is published/unpublished on
  // demand, the same as screen share — not merely enabled/disabled, so a
  // remote viewer's tile actually appears and disappears rather than
  // freezing on whatever frame was last sent.
  const toggleCamera = useCallback(async () => {
    if (!peer.current) return
    if (isCameraOff) {
      try {
        const videoDeviceId = loadDevicePrefs().videoInputId
        const media = await navigator.mediaDevices.getUserMedia({
          video: { deviceId: videoDeviceId ? { exact: videoDeviceId } : undefined },
        })
        const track = media.getVideoTracks()[0]
        if (!track) return
        local.current?.addTrack(track)
        peer.current.addTrack(track, local.current ?? media)
        knownKinds.current.set(track, 'camera')
        setIsCameraOff(false)
        setLocalStream(local.current)
        await renegotiate()
      } catch (error) { if (error instanceof Error && error.name !== 'NotAllowedError') setError(error.message) }
    } else {
      const track = local.current?.getVideoTracks()[0]
      if (!track) return
      const sender = peer.current.getSenders().find(candidate => candidate.track === track)
      if (sender) peer.current.removeTrack(sender)
      track.stop()
      local.current?.removeTrack(track)
      knownKinds.current.delete(track)
      setIsCameraOff(true)
      setLocalStream(local.current)
      await renegotiate()
    }
  }, [isCameraOff, renegotiate])

  // switchDevice live-swaps the mic or camera track on an ACTIVE call via
  // RTCRtpSender.replaceTrack — no renegotiation needed, unlike
  // addTrack/removeTrack. A no-op when there's no active call: the Devices
  // tab always saves the choice to devicePrefs regardless, so it still
  // takes effect on the next start()/toggleCamera() call.
  const switchDevice = useCallback(async (kind: 'audioinput' | 'videoinput', deviceId: string) => {
    if (!peer.current || !local.current) return
    const isAudio = kind === 'audioinput'
    // Sourced from local.current — which only ever holds mic+camera tracks,
    // never screen tracks (those live separately in screen.current) — and
    // matched below by exact track identity, not by kind: a default screen
    // share (see defaultScreenShareOptions, audio: true) adds a second
    // 'audio'-kind sender (tab/system audio) and a second 'video'-kind
    // sender (screen video) alongside the mic/camera ones, so matching by
    // kind alone on the whole peer connection could silently replace the
    // screen share's track instead of the mic/camera's.
    const oldTrack = isAudio ? local.current.getAudioTracks()[0] : local.current.getVideoTracks()[0]
    try {
      let newTrack: MediaStreamTrack | undefined
      if (isAudio) {
        // Routed through localAudio, not a bare getUserMedia call, so a
        // device switch while noise suppression is on keeps publishing a
        // processed track — and tears down the OLD device's raw capture
        // feeding the now-orphaned RNNoise graph — the same reasoning as
        // setNoiseSuppression's own localAudio+noiseCleanup pairing below.
        const prefs = loadDevicePrefs()
        noiseCleanup.current()
        const { stream, cleanup } = await localAudio(deviceId, prefs.noiseSuppression, prefs.noiseSuppression ? getAudioContext() : undefined)
        noiseCleanup.current = cleanup
        newTrack = stream.getAudioTracks()[0]
      } else {
        const media = await navigator.mediaDevices.getUserMedia({ video: { deviceId: { exact: deviceId } } })
        newTrack = media.getVideoTracks()[0]
      }
      if (!newTrack) return
      if (oldTrack) {
        const sender = peer.current.getSenders().find(s => s.track === oldTrack)
        if (sender) await sender.replaceTrack(newTrack)
        // Recorded in the same tick as the swap, same reasoning as
        // shareScreen/toggleCamera's own knownKinds.current.set calls: the
        // old track's entry would otherwise point buildTrackKinds at a
        // MediaStreamTrack object no transceiver holds any more, and the
        // freshly swapped-in track would have no recorded kind at all for
        // whichever renegotiation (a later toggleCamera, shareScreen, or
        // reconnect) comes next.
        knownKinds.current.delete(oldTrack)
        local.current.removeTrack(oldTrack)
        oldTrack.stop()
        local.current.addTrack(newTrack)
        knownKinds.current.set(newTrack, isAudio ? 'mic' : 'camera')
      } else {
        // No existing mic/camera track to swap (e.g. the camera is
        // currently off) — publish the new track the same way toggleCamera's
        // ON branch does (addTrack, record its kind, flip isCameraOff, then
        // renegotiate): peer.current.addTrack alone never reaches the peer,
        // since this file only renegotiates explicitly — there's no
        // negotiationneeded listener anywhere — so skipping renegotiate()
        // here would leave the new track stranded exactly as before, just
        // one step later. isCameraOff must flip too, or a subsequent
        // toggleCamera() call (still seeing isCameraOff === true) would
        // acquire and publish a second, duplicate video track.
        local.current.addTrack(newTrack)
        peer.current.addTrack(newTrack, local.current)
        knownKinds.current.set(newTrack, isAudio ? 'mic' : 'camera')
        if (!isAudio) setIsCameraOff(false)
        await renegotiate()
      }
      setLocalStream(local.current)
    } catch (error) { if (error instanceof Error && error.name !== 'NotAllowedError') setError(error.message) }
  }, [renegotiate])

  // setNoiseSuppression re-acquires the mic through (or back out of) RNNoise
  // and republishes it into the active call, same mechanism switchDevice
  // uses for a device change — noise suppression is really "switch to a
  // processed version of the same device". A no-op on the active call when
  // there isn't one; devicePrefs is still saved regardless, so it takes
  // effect on the next start().
  const setNoiseSuppression = useCallback(async (enabled: boolean) => {
    const prefs = loadDevicePrefs()
    saveDevicePrefs({ ...prefs, noiseSuppression: enabled })
    if (!peer.current || !local.current) return
    // Sourced from local.current and matched below by exact track identity,
    // not by kind — see switchDevice's own comment: a screen share's
    // tab/system audio is also an 'audio'-kind sender, so a kind-only
    // lookup across the whole peer connection could silently grab it
    // instead of the mic's own sender.
    const oldTrack = local.current.getAudioTracks()[0]
    try {
      noiseCleanup.current()
      const { stream: media, cleanup } = await localAudio(prefs.audioInputId, enabled, enabled ? getAudioContext() : undefined)
      noiseCleanup.current = cleanup
      const newTrack = media.getAudioTracks()[0]
      if (!newTrack) return
      if (oldTrack) {
        const sender = peer.current.getSenders().find(s => s.track === oldTrack)
        if (sender) await sender.replaceTrack(newTrack)
        // Recorded in the same tick as the swap, same reasoning as
        // switchDevice's own knownKinds bookkeeping.
        knownKinds.current.delete(oldTrack)
        local.current.removeTrack(oldTrack)
        oldTrack.stop()
        local.current.addTrack(newTrack)
        knownKinds.current.set(newTrack, 'mic')
      } else {
        // No existing mic track to swap (unlikely for audio specifically,
        // but handled the same way as switchDevice's own "no old track"
        // branch for consistency): publish via addTrack, record its kind,
        // then renegotiate() explicitly — this file has no
        // onnegotiationneeded listener, so peer.current.addTrack alone
        // never reaches the peer.
        local.current.addTrack(newTrack)
        peer.current.addTrack(newTrack, local.current)
        knownKinds.current.set(newTrack, 'mic')
        await renegotiate()
      }
      setLocalStream(local.current)
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось изменить шумоподавление')
    }
  }, [renegotiate])

  const moderate = useCallback(async (userID: string, action: 'mute' | 'unmute' | 'remove') => {
    if (!callID.current) return
    try {
      if (action === 'mute') await api.muteCallParticipant(callID.current, userID, sessionRef.current.access_token)
      else if (action === 'unmute') await api.unmuteCallParticipant(callID.current, userID, sessionRef.current.access_token)
      else await api.removeCallParticipant(callID.current, userID, sessionRef.current.access_token)
    } catch (error) { setError(error instanceof Error ? error.message : 'Не удалось изменить участника') }
  }, [])

  return { start, close, shareScreen, stopScreenShare, toggleMute, toggleCamera, switchDevice, setNoiseSuppression, moderate, reconnectNow, localStream, localScreenStream, remoteStreams, peers, error, isConnecting, isOpen, isMuted, isCameraOff, isSharingScreen, reconnectExhausted }
}

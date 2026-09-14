import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import type { Conversation, Session } from '../types'

type Signal = { type: string; sdp?: string; candidate?: RTCIceCandidateInit; message?: string; peers?: CallPeer[] }
export type RemoteStream = { id: string; stream: MediaStream; trackIDs: string[]; sourceStreamID?: string; kind?: string; ownerID?: string; firstSeenAt: number }

// staleStreamGraceMs guards against a benign race: the peers roster and the
// RTP that fires ontrack travel over separate transports (WS vs DTLS-SRTP)
// with no ordering guarantee between them, so a stream that is genuinely
// brand new can briefly be missing from the very first peers frame after it
// was created. Only prune an entry the roster no longer mentions once it has
// existed longer than this.
const staleStreamGraceMs = 1_500
export type CallPeer = { user_id: string; role: string; muted: boolean; tracks?: { track_id: string; stream_id: string; kind: string; label: string }[] }

// maxReconnectDelayMs caps the signalling socket's reconnect backoff, mirroring
// useRealtimeChat's message socket — the one place a dropped connection here
// already recovers on its own.
const maxReconnectDelayMs = 15_000

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

// Camera starts off: only the microphone is captured up front. toggleCamera
// acquires and publishes a camera track on demand.
async function localAudio() {
  const devices = await navigator.mediaDevices.enumerateDevices()
  if (!devices.some(device => device.kind === 'audioinput')) return new MediaStream()
  try { return await navigator.mediaDevices.getUserMedia({ audio: true }) } catch { return new MediaStream() }
}

function ownerIDFromForwardedStream(streamID: string) {
  const match = /^pm-([0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})-/i.exec(streamID)
  return match?.[1]
}

export function useCall(session: Session) {
  const peer = useRef<RTCPeerConnection | null>(null)
  const signal = useRef<WebSocket | null>(null)
  const local = useRef<MediaStream | null>(null)
  const screen = useRef<MediaStream | null>(null)
  const callID = useRef<string | null>(null)
  const sessionRef = useRef(session)
  useEffect(() => { sessionRef.current = session }, [session])
  const closedByClient = useRef(true)
  const reconnectAttempt = useRef(0)
  const reconnectTimer = useRef<number | undefined>(undefined)
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

  const [localStream, setLocalStream] = useState<MediaStream | null>(null)
  const [remoteStreams, setRemoteStreams] = useState<RemoteStream[]>([])
  const [peers, setPeers] = useState<CallPeer[]>([])
  const [error, setError] = useState('')
  const [isConnecting, setIsConnecting] = useState(false)
  const [isOpen, setIsOpen] = useState(false)
  const [isMuted, setIsMuted] = useState(false)
  const [isCameraOff, setIsCameraOff] = useState(true)
  const [isSharingScreen, setIsSharingScreen] = useState(false)

  const close = useCallback(async (end = false) => {
    closedByClient.current = true
    if (reconnectTimer.current) { window.clearTimeout(reconnectTimer.current); reconnectTimer.current = undefined }
    const id = callID.current
    signal.current?.close(); signal.current = null
    peer.current?.close(); peer.current = null
    screen.current?.getTracks().forEach(track => track.stop()); screen.current = null
    local.current?.getTracks().forEach(track => track.stop()); local.current = null
    setLocalStream(null); setRemoteStreams([]); setPeers([]); setIsOpen(false); setIsConnecting(false); setIsMuted(false); setIsCameraOff(true); setIsSharingScreen(false)
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
      // look up this track's kind. event.streams[0].id (the MediaStream the
      // track arrived on) does carry the SFU's id through; that's the only
      // thing worth matching camera/screen labels against.
      const sourceStreamID = event.streams[0]?.id
      setRemoteStreams(previous => {
        if (previous.some(item => item.id === event.track.id)) return previous
        return [...previous, { id: event.track.id, stream, trackIDs: [event.track.id], sourceStreamID, kind: event.track.kind, ownerID: ownerIDFromForwardedStream(sourceStreamID ?? ''), firstSeenAt: Date.now() }]
      })
    }

    if (local.current) local.current.getTracks().forEach(track => connection.addTrack(track, local.current!))
    if (screen.current) {
      const track = screen.current.getVideoTracks()[0]
      if (track) connection.addTrack(track, screen.current)
    }

    const socket = new WebSocket(`${join.media_ws_url}?ticket=${encodeURIComponent(join.ticket)}`)
    signal.current = socket
    socket.onopen = () => enqueue(async () => {
      const offer = await connection.createOffer()
      await connection.setLocalDescription(offer)
      const screenTrack = screen.current?.getVideoTracks()[0]
      const screenMid = screenTrack && connection.getTransceivers().find(candidate => candidate.sender.track === screenTrack)?.mid
      socket.send(JSON.stringify({ type: 'offer', sdp: offer.sdp, track_kinds: screenMid ? { [screenMid]: 'screen' } : undefined }))
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
        // Keyed by stream_id *and* kind: a camera video track and its mic
        // audio track publish on the same MediaStream (same stream_id), so
        // stream_id alone can't tell a dropped video track from a mic that
        // is still very much live on that same id.
        const liveTrackKeys = new Set(frame.peers.flatMap(peer => peer.tracks?.map(track => `${track.stream_id}:${track.kind}`) ?? []))
        const now = Date.now()
        setRemoteStreams(previous => previous.filter(entry =>
          !entry.sourceStreamID || liveTrackKeys.has(`${entry.sourceStreamID}:${entry.kind}`) || now - entry.firstSeenAt < staleStreamGraceMs))
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
  // indefinitely with capped backoff, same policy as the message socket —
  // the alternative is a call that silently stays dead after any blip
  // (a background tab throttling timers, a brief network drop) with no way
  // back short of hanging up and redialling.
  const scheduleReconnect = useCallback(() => {
    if (closedByClient.current || !callID.current) return
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

  const start = useCallback(async (conversation: Conversation) => {
    closedByClient.current = false
    reconnectAttempt.current = 0
    setError(''); setIsConnecting(true)
    try {
      const { call, join } = await api.startCall(conversation.id, sessionRef.current.access_token)
      callID.current = call.id
      const stream = await localAudio()
      local.current = stream; setLocalStream(stream)
      connectSignaling(join)
    } catch (error) { setError(error instanceof Error ? error.message : 'Не удалось начать звонок'); await close(false) }
  }, [close, connectSignaling])

  // renegotiate sends a fresh offer reflecting whatever the caller already
  // did to peer.current's senders (addTrack/removeTrack) — shareScreen,
  // stopScreenShare and toggleCamera all end with this instead of repeating
  // the offer/mid/track_kinds dance. newKinds declares the kind of any track
  // added in THIS round (mid isn't known until after the offer is created,
  // so it's resolved here, the same way the initial screen-share declaration
  // always worked).
  const renegotiate = useCallback((newKinds?: Map<MediaStreamTrack, string>) => enqueueNegotiation(async () => {
    const connection = peer.current
    const socket = signal.current
    if (!connection || !socket || socket.readyState !== WebSocket.OPEN) return
    const offer = await connection.createOffer()
    await connection.setLocalDescription(offer)
    let trackKinds: Record<string, string> | undefined
    if (newKinds?.size) {
      trackKinds = {}
      for (const transceiver of connection.getTransceivers()) {
        const track = transceiver.sender.track
        const kind = track && newKinds.get(track)
        if (kind && transceiver.mid) trackKinds[transceiver.mid] = kind
      }
    }
    socket.send(JSON.stringify({ type: 'offer', sdp: offer.sdp, track_kinds: trackKinds }))
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
    tracks.forEach(track => track.stop())
    screen.current = null
    setLocalStream(local.current)
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
      const audioTrack = display.getAudioTracks()[0]
      if (audioTrack) peer.current.addTrack(audioTrack, display)
      setLocalStream(display)
      setIsSharingScreen(true)
      track.onended = () => { if (screen.current === display) stopScreenShare() }
      // A MediaStream's id can't be renamed in a way that reaches the SDP the
      // browser generates, so the SFU can't infer "screen" from it (that used
      // to be attempted here and never actually worked). Declare the kind
      // explicitly by mid instead — the one signal that is actually reliable.
      // The tab/system audio track, when present, shares the screen's own
      // stream id (same source MediaStream), so it gets the same label.
      const kinds = new Map<MediaStreamTrack, string>([[track, 'screen']])
      if (audioTrack) kinds.set(audioTrack, 'screen')
      await renegotiate(kinds)
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
        const media = await navigator.mediaDevices.getUserMedia({ video: true })
        const track = media.getVideoTracks()[0]
        if (!track) return
        local.current?.addTrack(track)
        peer.current.addTrack(track, local.current ?? media)
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
      setIsCameraOff(true)
      setLocalStream(local.current)
      await renegotiate()
    }
  }, [isCameraOff, renegotiate])

  const moderate = useCallback(async (userID: string, action: 'mute' | 'unmute' | 'remove') => {
    if (!callID.current) return
    try {
      if (action === 'mute') await api.muteCallParticipant(callID.current, userID, sessionRef.current.access_token)
      else if (action === 'unmute') await api.unmuteCallParticipant(callID.current, userID, sessionRef.current.access_token)
      else await api.removeCallParticipant(callID.current, userID, sessionRef.current.access_token)
    } catch (error) { setError(error instanceof Error ? error.message : 'Не удалось изменить участника') }
  }, [])

  return { start, close, shareScreen, stopScreenShare, toggleMute, toggleCamera, moderate, localStream, remoteStreams, peers, error, isConnecting, isOpen, isMuted, isCameraOff, isSharingScreen }
}

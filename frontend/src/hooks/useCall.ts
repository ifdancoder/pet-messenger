import { useCallback, useRef, useState } from 'react'
import { api } from '../lib/api'
import type { Conversation, Session } from '../types'

type Signal = { type: string; sdp?: string; candidate?: RTCIceCandidateInit; message?: string; peers?: CallPeer[] }
export type RemoteStream = { id: string; stream: MediaStream; trackIDs: string[]; ownerID?: string }
export type CallPeer = { user_id: string; role: string; muted: boolean; tracks?: { track_id: string; kind: string; label: string }[] }

async function localMedia() {
  const devices = await navigator.mediaDevices.enumerateDevices()
  const audio = devices.some(device => device.kind === 'audioinput')
  const video = devices.some(device => device.kind === 'videoinput')
  if (!audio && !video) return new MediaStream()
  try { return await navigator.mediaDevices.getUserMedia({ audio, video }) } catch { return new MediaStream() }
}

function ownerIDFromForwardedStream(streamID: string) {
  const match = /^pm-([0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})-/i.exec(streamID)
  return match?.[1]
}

export function useCall(session: Session) {
  const peer = useRef<RTCPeerConnection | null>(null)
  const signal = useRef<WebSocket | null>(null)
  const local = useRef<MediaStream | null>(null)
  const fallbackRemote = useRef<MediaStream | null>(null)
  const callID = useRef<string | null>(null)
  const [localStream, setLocalStream] = useState<MediaStream | null>(null)
  const [remoteStreams, setRemoteStreams] = useState<RemoteStream[]>([])
  const [peers, setPeers] = useState<CallPeer[]>([])
  const [error, setError] = useState('')
  const [isConnecting, setIsConnecting] = useState(false)
  const [isOpen, setIsOpen] = useState(false)
  const [isMuted, setIsMuted] = useState(false)
  const [isCameraOff, setIsCameraOff] = useState(false)

  const close = useCallback(async (end = false) => {
    const id = callID.current
    signal.current?.close(); signal.current = null
    peer.current?.close(); peer.current = null
    local.current?.getTracks().forEach(track => track.stop()); local.current = null
    fallbackRemote.current = null
    setLocalStream(null); setRemoteStreams([]); setPeers([]); setIsOpen(false); setIsConnecting(false); setIsMuted(false); setIsCameraOff(false)
    callID.current = null
    if (end && id) await api.endCall(id, session.access_token).catch(() => undefined)
  }, [session.access_token])

  const start = useCallback(async (conversation: Conversation) => {
    setError(''); setIsConnecting(true)
    try {
      const { call, join } = await api.startCall(conversation.id, session.access_token)
      callID.current = call.id
      const connection = new RTCPeerConnection({ iceServers: join.ice_servers })
      peer.current = connection
      fallbackRemote.current = new MediaStream()
      const pendingCandidates: RTCIceCandidateInit[] = []
      let operations = Promise.resolve()
      const enqueue = (task: () => Promise<void>) => { operations = operations.then(task).catch(error => setError(error instanceof Error ? error.message : 'Ошибка соединения')) }
      connection.onconnectionstatechange = () => {
        switch (connection.connectionState) {
          case 'connected':
            setIsConnecting(false)
            setIsOpen(true)
            break
          case 'failed':
            setIsConnecting(false)
            setIsOpen(false)
            setError('Не удалось установить медиа-соединение')
            break
          case 'closed':
            setIsConnecting(false)
            setIsOpen(false)
            break
        }
      }
      connection.onicecandidate = event => { if (event.candidate) signal.current?.send(JSON.stringify({ type: 'candidate', candidate: event.candidate.toJSON() })) }
      connection.ontrack = event => {
        const stream = event.streams[0] ?? fallbackRemote.current
        if (!stream) return
        if (event.streams.length === 0 && !stream.getTracks().some(track => track.id === event.track.id)) stream.addTrack(event.track)
        setRemoteStreams(previous => {
          const known = previous.find(item => item.id === stream.id)
          if (!known) return [...previous, { id: stream.id, stream, trackIDs: [event.track.id], ownerID: ownerIDFromForwardedStream(stream.id) }]
          if (known.trackIDs.includes(event.track.id)) return previous
          return previous.map(item => item.id === stream.id ? { ...item, trackIDs: [...item.trackIDs, event.track.id] } : item)
        })
      }
      const stream = await localMedia()
      local.current = stream; setLocalStream(stream)
      stream.getTracks().forEach(track => connection.addTrack(track, stream))
      const socket = new WebSocket(`${join.media_ws_url}?ticket=${encodeURIComponent(join.ticket)}`)
      signal.current = socket
      socket.onopen = () => enqueue(async () => {
        const offer = await connection.createOffer()
        await connection.setLocalDescription(offer)
        socket.send(JSON.stringify({ type: 'offer', sdp: offer.sdp }))
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
        } else if (frame.type === 'peers' && frame.peers) setPeers(frame.peers)
        else if (frame.type === 'removed') await close(false)
        else if (frame.type === 'error') setError(frame.message ?? 'Ошибка звонка')
        if (connection.remoteDescription) for (const candidate of pendingCandidates.splice(0)) await connection.addIceCandidate(candidate)
      })
      socket.onclose = event => {
        if (signal.current !== socket) return
        setIsOpen(false)
        setIsConnecting(false)
        if (event.code !== 1000) setError(`Сигнальное соединение закрыто (${event.code})`)
      }
    } catch (error) { setError(error instanceof Error ? error.message : 'Не удалось начать звонок'); await close(false) }
  }, [close, session.access_token])

  const shareScreen = useCallback(async () => {
    if (!peer.current) return
    try {
      const display = await navigator.mediaDevices.getDisplayMedia({ video: true })
      const track = display.getVideoTracks()[0]
      if (!track) return
      const stream = new MediaStream([track]); Object.defineProperty(stream, 'id', { value: `screen-${crypto.randomUUID()}` })
      peer.current.addTrack(track, stream)
      const offer = await peer.current.createOffer(); await peer.current.setLocalDescription(offer)
      signal.current?.send(JSON.stringify({ type: 'offer', sdp: offer.sdp }))
      setLocalStream(stream)
    } catch (error) { if (error instanceof Error && error.name !== 'NotAllowedError') setError(error.message) }
  }, [])

  const toggleMute = useCallback(() => {
    const tracks = local.current?.getAudioTracks() ?? []
    if (!tracks.length) return
    const muted = tracks.some(track => track.enabled)
    tracks.forEach(track => { track.enabled = !muted })
    setIsMuted(muted)
  }, [])

  const toggleCamera = useCallback(() => {
    const tracks = local.current?.getVideoTracks() ?? []
    if (!tracks.length) return
    const cameraOff = tracks.some(track => track.enabled)
    tracks.forEach(track => { track.enabled = !cameraOff })
    setIsCameraOff(cameraOff)
  }, [])

  const moderate = useCallback(async (userID: string, action: 'mute' | 'unmute' | 'remove') => {
    if (!callID.current) return
    try {
      if (action === 'mute') await api.muteCallParticipant(callID.current, userID, session.access_token)
      else if (action === 'unmute') await api.unmuteCallParticipant(callID.current, userID, session.access_token)
      else await api.removeCallParticipant(callID.current, userID, session.access_token)
    } catch (error) { setError(error instanceof Error ? error.message : 'Не удалось изменить участника') }
  }, [session.access_token])

  return { start, close, shareScreen, toggleMute, toggleCamera, moderate, localStream, remoteStreams, peers, error, isConnecting, isOpen, isMuted, isCameraOff }
}

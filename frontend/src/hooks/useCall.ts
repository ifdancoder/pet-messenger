import { useCallback, useRef, useState } from 'react'
import { api } from '../lib/api'
import type { Conversation, Session } from '../types'

type Signal = { type: string; sdp?: string; candidate?: RTCIceCandidateInit; message?: string; peers?: CallPeer[] }
type StreamEntry = { id: string; stream: MediaStream }
export type CallPeer = { user_id: string; role: string; muted: boolean }

async function localMedia() {
  const devices = await navigator.mediaDevices.enumerateDevices()
  const audio = devices.some(device => device.kind === 'audioinput')
  const video = devices.some(device => device.kind === 'videoinput')
  if (!audio && !video) return new MediaStream()
  try { return await navigator.mediaDevices.getUserMedia({ audio, video }) } catch { return new MediaStream() }
}

export function useCall(session: Session) {
  const peer = useRef<RTCPeerConnection | null>(null)
  const signal = useRef<WebSocket | null>(null)
  const local = useRef<MediaStream | null>(null)
  const callID = useRef<string | null>(null)
  const [localStream, setLocalStream] = useState<MediaStream | null>(null)
  const [remoteStreams, setRemoteStreams] = useState<StreamEntry[]>([])
  const [peers, setPeers] = useState<CallPeer[]>([])
  const [error, setError] = useState('')
  const [isConnecting, setIsConnecting] = useState(false)
  const [isOpen, setIsOpen] = useState(false)

  const close = useCallback(async (end = false) => {
    const id = callID.current
    signal.current?.close(); signal.current = null
    peer.current?.close(); peer.current = null
    local.current?.getTracks().forEach(track => track.stop()); local.current = null
    setLocalStream(null); setRemoteStreams([]); setPeers([]); setIsOpen(false); setIsConnecting(false)
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
      const pendingCandidates: RTCIceCandidateInit[] = []
      let operations = Promise.resolve()
      const enqueue = (task: () => Promise<void>) => { operations = operations.then(task).catch(error => setError(error instanceof Error ? error.message : 'Ошибка соединения')) }
      connection.onicecandidate = event => { if (event.candidate) signal.current?.send(JSON.stringify({ type: 'candidate', candidate: event.candidate.toJSON() })) }
      connection.ontrack = event => {
        const stream = event.streams[0]
        if (stream) setRemoteStreams(previous => previous.some(item => item.id === stream.id) ? previous : [...previous, { id: stream.id, stream }])
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
        setIsConnecting(false); setIsOpen(true)
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
      socket.onclose = () => { if (signal.current === socket) { setIsOpen(false); setIsConnecting(false) } }
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

  const moderate = useCallback(async (userID: string, action: 'mute' | 'unmute' | 'remove') => {
    if (!callID.current) return
    try {
      if (action === 'mute') await api.muteCallParticipant(callID.current, userID, session.access_token)
      else if (action === 'unmute') await api.unmuteCallParticipant(callID.current, userID, session.access_token)
      else await api.removeCallParticipant(callID.current, userID, session.access_token)
    } catch (error) { setError(error instanceof Error ? error.message : 'Не удалось изменить участника') }
  }, [session.access_token])

  return { start, close, shareScreen, moderate, localStream, remoteStreams, peers, error, isConnecting, isOpen }
}

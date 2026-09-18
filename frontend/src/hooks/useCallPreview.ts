import { useCallback, useRef, useState } from 'react'
import { api } from '../lib/api'
import type { RemoteStream } from './useCall'
import { entryMatchesTrack, ownerIDFromForwardedStream } from './useCall'
import type { Session } from '../types'

// useCallPreview is deliberately the leanest possible receive-only
// connection: no getUserMedia, no local track ever added, no reconnect
// logic, no mute/camera/screen controls. It exists purely to let someone
// who hasn't joined a call yet see a live preview of whoever's camera or
// screen share is already up — joining the room as a "viewer" (see
// domaincall.RoleViewer), a role the SFU already enforces as
// receive-only (internal/adapter/sfu/forward.go) but that nothing in the
// app has ever actually requested until now.
export function useCallPreview(session: Session) {
  const peer = useRef<RTCPeerConnection | null>(null)
  const signal = useRef<WebSocket | null>(null)
  const startedFor = useRef<string | null>(null)
  const [remoteStreams, setRemoteStreams] = useState<RemoteStream[]>([])

  const stop = useCallback(() => {
    startedFor.current = null
    signal.current?.close()
    signal.current = null
    peer.current?.close()
    peer.current = null
    setRemoteStreams([])
  }, [])

  const start = useCallback((callID: string) => {
    if (startedFor.current === callID) return
    stop()
    startedFor.current = callID
    void (async () => {
      try {
        const { join } = await api.joinCall(callID, session.access_token, 'viewer')
        if (startedFor.current !== callID) return // stop() ran while the request was in flight
        const connection = new RTCPeerConnection({ iceServers: join.ice_servers })
        peer.current = connection
        // Receive-only: no local tracks, so the SFU only ever forwards
        // publishers' media to this connection, never the other way.
        connection.addTransceiver('audio', { direction: 'recvonly' })
        connection.addTransceiver('video', { direction: 'recvonly' })
        const pendingCandidates: RTCIceCandidateInit[] = []
        connection.onicecandidate = event => { if (event.candidate) signal.current?.send(JSON.stringify({ type: 'candidate', candidate: event.candidate.toJSON() })) }
        connection.ontrack = event => {
          const stream = new MediaStream([event.track])
          const mid = event.transceiver?.mid ?? undefined
          const sourceStreamID = event.streams[0]?.id
          setRemoteStreams(previous => {
            if (previous.some(item => item.id === event.track.id)) return previous
            return [...previous, { id: event.track.id, stream, trackIDs: [event.track.id], sourceStreamID, mid, kind: event.track.kind, ownerID: ownerIDFromForwardedStream(sourceStreamID ?? ''), firstSeenAt: Date.now() }]
          })
        }
        const socket = new WebSocket(`${join.media_ws_url}?ticket=${encodeURIComponent(join.ticket)}`)
        signal.current = socket
        socket.onopen = async () => {
          const offer = await connection.createOffer()
          await connection.setLocalDescription(offer)
          socket.send(JSON.stringify({ type: 'offer', sdp: offer.sdp }))
        }
        socket.onmessage = async event => {
          const frame = JSON.parse(String(event.data)) as { type: string; sdp?: string; candidate?: RTCIceCandidateInit; peers?: { tracks?: { kind: string; mid?: string; stream_id: string }[] }[] }
          if (frame.type === 'answer' && frame.sdp) await connection.setRemoteDescription({ type: 'answer', sdp: frame.sdp })
          else if (frame.type === 'candidate' && frame.candidate) {
            if (connection.remoteDescription) await connection.addIceCandidate(frame.candidate); else pendingCandidates.push(frame.candidate)
          } else if (frame.type === 'peers' && frame.peers) {
            const allTracks = frame.peers.flatMap(p => p.tracks ?? [])
            setRemoteStreams(previous => previous.filter(entry => allTracks.some(track => track.kind === entry.kind && entryMatchesTrack(entry, track))))
          }
          if (connection.remoteDescription) for (const candidate of pendingCandidates.splice(0)) await connection.addIceCandidate(candidate)
        }
      } catch {
        // A failed preview connection is not worth surfacing as an error —
        // CallPreviewBar still shows the participant list from /calls/live
        // regardless; the live thumbnails are a nice-to-have on top of that.
        stop()
      }
    })()
  }, [session.access_token, stop])

  return { start, stop, remoteStreams }
}

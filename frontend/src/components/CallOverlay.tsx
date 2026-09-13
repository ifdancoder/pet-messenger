import { useEffect, useRef } from 'react'
import type { CallPeer } from '../hooks/useCall'
import { Button, Notice, Spinner } from './ui'

type StreamEntry = { id: string; stream: MediaStream }
function VideoTile({ stream, muted, label }: { stream: MediaStream; muted?: boolean; label: string }) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  return <figure className="call-video"><video ref={video} autoPlay playsInline muted={muted} /><figcaption>{label}</figcaption></figure>
}
function ParticipantSection({ title, streams, muted }: { title: string; streams: StreamEntry[]; muted?: boolean }) {
  return <article className="call-participant"><header><span>{title}</span><small>{streams.length > 1 ? 'видео и демонстрация' : 'медиа'}</small></header><div className="call-participant__media">{streams.map((entry, index) => <VideoTile key={entry.id} stream={entry.stream} muted={muted} label={index === 0 ? 'Камера' : 'Демонстрация экрана'} />)}</div></article>
}

type Props = { localStream: MediaStream | null; remoteStreams: StreamEntry[]; peers: CallPeer[]; currentUserID: string; canModerate: boolean; error: string; isConnecting: boolean; isConnected: boolean; isMuted: boolean; isCameraOff: boolean; onShare: () => Promise<void>; onToggleMute: () => void; onToggleCamera: () => void; onModerate: (userID: string, action: 'mute' | 'unmute' | 'remove') => Promise<void>; onEnd: () => Promise<void> }
export function CallOverlay({ localStream, remoteStreams, peers, currentUserID, canModerate, error, isConnecting, isConnected, isMuted, isCameraOff, onShare, onToggleMute, onToggleCamera, onModerate, onEnd }: Props) {
  const otherPeers = peers.filter(peer => peer.user_id !== currentUserID)
  const title = isConnected ? 'Соединение установлено' : isConnecting ? 'Подключаемся…' : 'Соединение потеряно'
  const local = localStream ? [{ id: 'local', stream: localStream }] : []
  return <section className="call-overlay" aria-label="Звонок"><header><div><span className="call-label">Звонок</span><h2>{title}</h2></div><Button variant="danger" onClick={() => void onEnd()}>Завершить</Button></header>{error ? <Notice>{error}</Notice> : null}<div className="call-grid">{local.length ? <ParticipantSection title="Вы" streams={local} muted /> : null}{remoteStreams.map((stream, index) => <ParticipantSection key={stream.id} title={otherPeers[index] ? `Участник ${index + 1}` : `Собеседник ${index + 1}`} streams={[stream]} />)}{isConnecting ? <div className="call-waiting"><Spinner /><span>Настраиваем защищённое соединение</span></div> : null}{isConnected && remoteStreams.length === 0 ? <div className="call-waiting">{otherPeers.length ? 'Собеседник подключён — ждём медиа' : 'Ожидаем собеседника'}</div> : null}</div><footer><Button variant="secondary" onClick={onToggleMute}>{isMuted ? 'Включить микрофон' : 'Выключить микрофон'}</Button><Button variant="secondary" onClick={onToggleCamera}>{isCameraOff ? 'Включить камеру' : 'Выключить камеру'}</Button><Button variant="secondary" onClick={() => void onShare()}>Показать экран</Button>{canModerate ? otherPeers.map(peer => <div className="call-peer" key={peer.user_id}><span>Участник · {peer.muted ? 'без звука' : 'со звуком'}</span><Button variant="ghost" onClick={() => void onModerate(peer.user_id, peer.muted ? 'unmute' : 'mute')}>{peer.muted ? 'Включить звук' : 'Выключить звук'}</Button><Button variant="danger" onClick={() => void onModerate(peer.user_id, 'remove')}>Исключить</Button></div>) : null}</footer></section>
}

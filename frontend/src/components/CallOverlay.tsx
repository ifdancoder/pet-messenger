import { useEffect, useRef } from 'react'
import { Button, Notice, Spinner } from './ui'

type StreamEntry = { id: string; stream: MediaStream }
function VideoTile({ stream, muted, label }: { stream: MediaStream; muted?: boolean; label: string }) {
  const video = useRef<HTMLVideoElement>(null)
  useEffect(() => { if (video.current) video.current.srcObject = stream }, [stream])
  return <figure className="call-video"><video ref={video} autoPlay playsInline muted={muted} /><figcaption>{label}</figcaption></figure>
}

type Props = { localStream: MediaStream | null; remoteStreams: StreamEntry[]; error: string; isConnecting: boolean; onShare: () => Promise<void>; onEnd: () => Promise<void> }
export function CallOverlay({ localStream, remoteStreams, error, isConnecting, onShare, onEnd }: Props) {
  return <section className="call-overlay" aria-label="Звонок"><header><div><span className="call-label">Звонок</span><h2>{isConnecting ? 'Подключаемся…' : 'Соединение установлено'}</h2></div><Button variant="danger" onClick={() => void onEnd()}>Завершить</Button></header>{error ? <Notice>{error}</Notice> : null}<div className="call-grid">{localStream ? <VideoTile stream={localStream} muted label="Вы" /> : null}{remoteStreams.map((stream, index) => <VideoTile key={stream.id} stream={stream.stream} label={`Участник ${index + 1}`} />)}{isConnecting ? <div className="call-waiting"><Spinner /><span>Настраиваем защищённое соединение</span></div> : null}{!isConnecting && remoteStreams.length === 0 ? <div className="call-waiting">Ожидаем собеседника</div> : null}</div><footer><Button variant="secondary" onClick={() => void onShare()}>Показать экран</Button></footer></section>
}

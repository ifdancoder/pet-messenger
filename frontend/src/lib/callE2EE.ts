type TransformableSender = RTCRtpSender & { transform?: unknown }
type TransformableReceiver = RTCRtpReceiver & { transform?: unknown }
type ScriptTransformConstructor = new (worker: Worker, options: Record<string, string>) => unknown

function base64url(data: Uint8Array) {
  let binary = ''
  data.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '')
}

export type CallE2EETransforms = {
  encrypt: (sender: RTCRtpSender) => void
  decrypt: (receiver: RTCRtpReceiver, mediaKind?: string) => void
  close: () => void
}

export function createCallE2EETransforms(callID: string, key: Uint8Array): CallE2EETransforms {
  const Constructor = (window as typeof window & { RTCRtpScriptTransform?: ScriptTransformConstructor }).RTCRtpScriptTransform
  if (!Constructor) throw new Error('Браузер не поддерживает E2EE медиапотоков')
  const worker = new Worker(new URL('../workers/callE2EEWorker.ts', import.meta.url), { type: 'module' })
  const encodedKey = base64url(key)
  const encrypted = new WeakSet<RTCRtpSender>()
  const decrypted = new WeakSet<RTCRtpReceiver>()
  return {
    encrypt(sender) {
      if (encrypted.has(sender)) return
      // Custom encoded transforms currently corrupt Opus on some browser/SFU
      // combinations even when the frame is restored byte-for-byte. Keep
      // audio on WebRTC's native DTLS-SRTP path until native SFrame audio
      // packetization is available; video still uses app-level E2EE below.
      if (sender.track?.kind === 'audio') return
      ;(sender as TransformableSender).transform = new Constructor(worker, {
        operation: 'encrypt', key: encodedKey, callID, mediaKind: sender.track?.kind ?? '',
      })
      encrypted.add(sender)
    },
    decrypt(receiver, mediaKind = receiver.track?.kind ?? '') {
      if (decrypted.has(receiver)) return
      if (mediaKind === 'audio' || receiver.track?.kind === 'audio') return
      ;(receiver as TransformableReceiver).transform = new Constructor(worker, {
        operation: 'decrypt', key: encodedKey, callID, mediaKind,
      })
      decrypted.add(receiver)
    },
    close() { worker.terminate() },
  }
}

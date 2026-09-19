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
  decrypt: (receiver: RTCRtpReceiver) => void
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
      ;(sender as TransformableSender).transform = new Constructor(worker, { operation: 'encrypt', key: encodedKey, callID })
      encrypted.add(sender)
    },
    decrypt(receiver) {
      if (decrypted.has(receiver)) return
      ;(receiver as TransformableReceiver).transform = new Constructor(worker, { operation: 'decrypt', key: encodedKey, callID })
      decrypted.add(receiver)
    },
    close() { worker.terminate() },
  }
}


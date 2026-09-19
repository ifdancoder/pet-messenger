type TransformableSender = RTCRtpSender & { transform?: unknown }
type TransformableReceiver = RTCRtpReceiver & { transform?: unknown }
type ScriptTransformConstructor = new (worker: Worker, options: Record<string, string>) => unknown
type NativeSFrameEncryptor = {
  setEncryptionKey: (key: CryptoKey, keyID: number) => Promise<void>
}
type NativeSFrameDecryptor = {
  addDecryptionKey: (key: CryptoKey, keyID: number) => Promise<void>
}
type NativeSFrameEncryptorConstructor = new (options: { cipherSuite: string; type: 'per-frame' }) => NativeSFrameEncryptor
type NativeSFrameDecryptorConstructor = new (options: { cipherSuite: string }) => NativeSFrameDecryptor

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
  const runtime = window as typeof window & {
    RTCRtpScriptTransform?: ScriptTransformConstructor
    RTCRtpSFrameEncryptor?: NativeSFrameEncryptorConstructor
    RTCRtpSFrameDecryptor?: NativeSFrameDecryptorConstructor
  }
  const Constructor = runtime.RTCRtpScriptTransform
  const NativeEncryptor = runtime.RTCRtpSFrameEncryptor
  const NativeDecryptor = runtime.RTCRtpSFrameDecryptor
  if (!Constructor && (!NativeEncryptor || !NativeDecryptor)) throw new Error('Браузер не поддерживает E2EE медиапотоков')
  const worker = Constructor ? new Worker(new URL('../workers/callE2EEWorker.ts', import.meta.url), { type: 'module' }) : null
  const encodedKey = base64url(key)
  const keyBytes = new Uint8Array(key.byteLength)
  keyBytes.set(key)
  const nativeKey = crypto.subtle.importKey('raw', keyBytes.buffer, { name: 'AES-GCM' }, false, ['encrypt', 'decrypt'])
  const encrypted = new WeakSet<RTCRtpSender>()
  const decrypted = new WeakSet<RTCRtpReceiver>()
  const installNativeAudioSender = (sender: RTCRtpSender) => {
    if (!NativeEncryptor) return
    try {
      const transform = new NativeEncryptor({ cipherSuite: 'AES_256_GCM_SHA512_128', type: 'per-frame' })
      ;(sender as TransformableSender).transform = transform
      void nativeKey.then(keyValue => transform.setEncryptionKey(keyValue, 0)).catch(() => undefined)
    } catch { /* unsupported implementation: leave native DTLS-SRTP audio untouched */ }
  }
  const installNativeAudioReceiver = (receiver: RTCRtpReceiver) => {
    if (!NativeDecryptor) return
    try {
      const transform = new NativeDecryptor({ cipherSuite: 'AES_256_GCM_SHA512_128' })
      ;(receiver as TransformableReceiver).transform = transform
      void nativeKey.then(keyValue => transform.addDecryptionKey(keyValue, 0)).catch(() => undefined)
    } catch { /* unsupported implementation: leave native DTLS-SRTP audio untouched */ }
  }
  return {
    encrypt(sender) {
      if (encrypted.has(sender)) return
      if (sender.track?.kind === 'audio') {
        installNativeAudioSender(sender)
        encrypted.add(sender)
        return
      }
      if (!Constructor || !worker) return
      ;(sender as TransformableSender).transform = new Constructor(worker, {
        operation: 'encrypt', key: encodedKey, callID, mediaKind: sender.track?.kind ?? '',
      })
      encrypted.add(sender)
    },
    decrypt(receiver, mediaKind = receiver.track?.kind ?? '') {
      if (decrypted.has(receiver)) return
      if (mediaKind === 'audio' || receiver.track?.kind === 'audio') {
        installNativeAudioReceiver(receiver)
        decrypted.add(receiver)
        return
      }
      if (!Constructor || !worker) return
      ;(receiver as TransformableReceiver).transform = new Constructor(worker, {
        operation: 'decrypt', key: encodedKey, callID, mediaKind,
      })
      decrypted.add(receiver)
    },
    close() { worker?.terminate() },
  }
}

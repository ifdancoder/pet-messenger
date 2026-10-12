type EncodedFrame = {
  data: ArrayBuffer
  timestamp?: number
  type?: 'key' | 'delta'
  getMetadata?: () => { mimeType?: string }
}
type TransformEvent = Event & {
  transformer: {
    readable: ReadableStream<EncodedFrame>
    writable: WritableStream<EncodedFrame>
    options: { operation: 'encrypt' | 'decrypt'; key: string; callID: string; mediaKind?: string }
  }
}

const marker = new Uint8Array([0x50, 0x4d, 0x45, 0x01])
const encoder = new TextEncoder()

function fromBase64url(value: string) {
  const normalized = value.replaceAll('-', '+').replaceAll('_', '/')
  const padded = normalized + '='.repeat((4 - normalized.length % 4) % 4)
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  return bytes
}

function cryptoOffset(frame: EncodedFrame, data: Uint8Array, mediaKind?: string) {
  const mimeType = frame.getMetadata?.().mimeType?.toLowerCase()
  // The transform is installed on a concrete RTCRtpSender/Receiver, so its
  // media kind is authoritative. Audio encoded frames must stay opaque: a
  // marker/nonce/tag changes their length, so retaining Opus framing bytes
  // would leave stale frame-size fields in aggregated packets and produce
  // audible corruption. The WebRTC packetizer owns the codec framing here.
  if (mediaKind === 'audio' || mimeType?.startsWith('audio/')) return 0
  // This SFU deliberately negotiates VP8 as its sole video codec. Preserve
  // the bytes the packetizer needs to retain key/delta frame structure.
  if (mimeType === 'video/vp8' || frame.type !== undefined) {
    return Math.min(frame.type === 'key' ? 10 : 3, data.length)
  }
  return 0
}

function startsWithMarker(data: Uint8Array, offset: number) {
  return marker.every((byte, index) => data[offset + index] === byte)
}

function authenticatedData(callID: string, clearPrefix: Uint8Array) {
  const context = encoder.encode(`pet-messenger:media-frame:v1:${callID}`)
  const output = new Uint8Array(context.length + clearPrefix.length)
  output.set(context)
  output.set(clearPrefix, context.length)
  return output
}

async function audioCounter(callID: string, timestamp: number | undefined) {
  const context = encoder.encode(`pet-messenger:audio-frame:v2:${callID}:${timestamp ?? 0}`)
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', context))
  return digest.slice(0, 16)
}

const workerScope = self as typeof self & { onrtctransform: ((event: TransformEvent) => void) | null }
workerScope.onrtctransform = event => {
  const { operation, key, callID, mediaKind } = event.transformer.options
  const rawKey = fromBase64url(key)
  const cryptoKey = crypto.subtle.importKey('raw', rawKey, { name: 'AES-GCM' }, false, operation === 'encrypt' ? ['encrypt'] : ['decrypt'])
  const audioCryptoKey = crypto.subtle.importKey('raw', rawKey, { name: 'AES-CTR' }, false, operation === 'encrypt' ? ['encrypt'] : ['decrypt'])
  const transform = new TransformStream<EncodedFrame, EncodedFrame>({
    async transform(frame, controller) {
      try {
        if (operation === 'encrypt') {
          const input = new Uint8Array(frame.data)
          if (mediaKind === 'audio' || frame.getMetadata?.().mimeType?.toLowerCase().startsWith('audio/')) {
            const counter = await audioCounter(callID, frame.timestamp)
            frame.data = await crypto.subtle.encrypt(
              { name: 'AES-CTR', counter, length: 64 },
              await audioCryptoKey,
              input,
            )
            controller.enqueue(frame)
            return
          }
          const offset = cryptoOffset(frame, input, mediaKind)
          const clearPrefix = input.slice(0, offset)
          const nonce = crypto.getRandomValues(new Uint8Array(12))
          const ciphertext = new Uint8Array(await crypto.subtle.encrypt(
            { name: 'AES-GCM', iv: nonce, additionalData: authenticatedData(callID, clearPrefix) },
            await cryptoKey,
            input.slice(offset),
          ))
          const output = new Uint8Array(offset + marker.length + nonce.length + ciphertext.length)
          output.set(clearPrefix)
          output.set(marker, offset)
          output.set(nonce, offset + marker.length)
          output.set(ciphertext, offset + marker.length + nonce.length)
          frame.data = output.buffer
          controller.enqueue(frame)
          return
        }
        const input = new Uint8Array(frame.data)
        if (mediaKind === 'audio' || frame.getMetadata?.().mimeType?.toLowerCase().startsWith('audio/')) {
          const counter = await audioCounter(callID, frame.timestamp)
          frame.data = await crypto.subtle.decrypt(
            { name: 'AES-CTR', counter, length: 64 },
            await audioCryptoKey,
            input,
          )
          controller.enqueue(frame)
          return
        }
        const offset = cryptoOffset(frame, input, mediaKind)
        if (!startsWithMarker(input, offset) || input.length < offset + marker.length + 12 + 16) return
        const clearPrefix = input.slice(0, offset)
        const nonceStart = offset + marker.length
        const nonce = input.slice(nonceStart, nonceStart + 12)
        const plaintext = new Uint8Array(await crypto.subtle.decrypt(
          { name: 'AES-GCM', iv: nonce, additionalData: authenticatedData(callID, clearPrefix) },
          await cryptoKey,
          input.slice(nonceStart + 12),
        ))
        const output = new Uint8Array(clearPrefix.length + plaintext.length)
        output.set(clearPrefix)
        output.set(plaintext, clearPrefix.length)
        frame.data = output.buffer
        controller.enqueue(frame)
      } catch {
        // Authentication failure means the SFU or network supplied a frame
        // that was not produced with this call key. Dropping it is safer than
        // forwarding corrupted media into the decoder.
      }
    },
  })
  void event.transformer.readable.pipeThrough(transform).pipeTo(event.transformer.writable)
}

export {}

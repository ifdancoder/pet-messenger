type EncodedFrame = { data: ArrayBuffer; type?: 'key' | 'delta' }
type TransformEvent = Event & {
  transformer: {
    readable: ReadableStream<EncodedFrame>
    writable: WritableStream<EncodedFrame>
    options: { operation: 'encrypt' | 'decrypt'; key: string; callID: string }
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

function cryptoOffset(frame: EncodedFrame, byteLength: number) {
  // The WebRTC packetizer/depacketizer consumes a small codec header before
  // the script transform runs on the other side. Opus needs its one-byte TOC;
  // VP8 needs its uncompressed key/delta header. Encrypting those bytes makes
  // the browser infer the wrong frame shape and produces audible corruption.
  const wanted = frame.type === 'key' ? 10 : frame.type === 'delta' ? 3 : 1
  return Math.min(wanted, byteLength)
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

const workerScope = self as typeof self & { onrtctransform: ((event: TransformEvent) => void) | null }
workerScope.onrtctransform = event => {
  const { operation, key, callID } = event.transformer.options
  const cryptoKey = crypto.subtle.importKey('raw', fromBase64url(key), { name: 'AES-GCM' }, false, operation === 'encrypt' ? ['encrypt'] : ['decrypt'])
  const transform = new TransformStream<EncodedFrame, EncodedFrame>({
    async transform(frame, controller) {
      try {
        if (operation === 'encrypt') {
          const input = new Uint8Array(frame.data)
          const offset = cryptoOffset(frame, input.length)
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
        const offset = cryptoOffset(frame, input.length)
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

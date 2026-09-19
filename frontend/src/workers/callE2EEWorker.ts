type EncodedFrame = { data: ArrayBuffer }
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

function startsWithMarker(data: Uint8Array) {
  return marker.every((byte, index) => data[index] === byte)
}

const workerScope = self as typeof self & { onrtctransform: ((event: TransformEvent) => void) | null }
workerScope.onrtctransform = event => {
  const { operation, key, callID } = event.transformer.options
  const cryptoKey = crypto.subtle.importKey('raw', fromBase64url(key), { name: 'AES-GCM' }, false, operation === 'encrypt' ? ['encrypt'] : ['decrypt'])
  const additionalData = encoder.encode(`pet-messenger:media-frame:v1:${callID}`)
  const transform = new TransformStream<EncodedFrame, EncodedFrame>({
    async transform(frame, controller) {
      try {
        if (operation === 'encrypt') {
          const nonce = crypto.getRandomValues(new Uint8Array(12))
          const ciphertext = new Uint8Array(await crypto.subtle.encrypt(
            { name: 'AES-GCM', iv: nonce, additionalData }, await cryptoKey, frame.data,
          ))
          const output = new Uint8Array(marker.length + nonce.length + ciphertext.length)
          output.set(marker)
          output.set(nonce, marker.length)
          output.set(ciphertext, marker.length + nonce.length)
          frame.data = output.buffer
          controller.enqueue(frame)
          return
        }
        const input = new Uint8Array(frame.data)
        if (!startsWithMarker(input) || input.length < marker.length + 12 + 16) return
        const nonce = input.slice(marker.length, marker.length + 12)
        frame.data = await crypto.subtle.decrypt(
          { name: 'AES-GCM', iv: nonce, additionalData }, await cryptoKey, input.slice(marker.length + 12),
        )
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

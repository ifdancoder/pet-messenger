type EncodedFrame = {
  data: ArrayBuffer
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

function opusSizeFieldLength(data: Uint8Array, offset: number) {
  if (offset >= data.length) return 0
  return data[offset] < 252 || offset + 1 >= data.length ? 1 : 2
}

// Opus packets can contain several frames. In that case the framing header
// extends beyond the TOC byte and carries a frame count, padding length and
// (for VBR) individual frame sizes. The browser packetizer reads this after
// the script transform, before RTP is produced, so encrypting any part of it
// makes otherwise valid audio intermittently sound like digital crackle.
function opusHeaderLength(data: Uint8Array) {
  if (!data.length) return 0
  const code = data[0] & 0x03
  if (code < 2) return 1
  if (code === 2) return Math.min(data.length, 1 + opusSizeFieldLength(data, 1))
  if (data.length < 2) return data.length

  const frameCountByte = data[1]
  const frameCount = frameCountByte & 0x3f
  const hasPadding = (frameCountByte & 0x40) !== 0
  const isVBR = (frameCountByte & 0x80) !== 0
  let offset = 2
  if (hasPadding) {
    while (offset < data.length) {
      const paddingByte = data[offset++]
      if (paddingByte !== 255) break
    }
  }
  if (isVBR) {
    for (let index = 0; index < frameCount - 1 && offset < data.length; index += 1) {
      offset += opusSizeFieldLength(data, offset)
    }
  }
  return Math.min(offset, data.length)
}

function cryptoOffset(frame: EncodedFrame, data: Uint8Array, mediaKind?: string) {
  const mimeType = frame.getMetadata?.().mimeType?.toLowerCase()
  // The transform is installed on a concrete RTCRtpSender/Receiver, so its
  // media kind is authoritative. Do not infer audio from frame.type: audio
  // frames have no key/delta type in the API, but some browser versions still
  // expose a legacy value there, which made one receive direction use VP8's
  // ten-byte header and turn valid Opus into noise.
  if (mediaKind === 'audio' || mimeType === 'audio/opus' || (!mediaKind && !mimeType && frame.type === undefined)) return opusHeaderLength(data)
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

const workerScope = self as typeof self & { onrtctransform: ((event: TransformEvent) => void) | null }
workerScope.onrtctransform = event => {
  const { operation, key, callID, mediaKind } = event.transformer.options
  const cryptoKey = crypto.subtle.importKey('raw', fromBase64url(key), { name: 'AES-GCM' }, false, operation === 'encrypt' ? ['encrypt'] : ['decrypt'])
  const transform = new TransformStream<EncodedFrame, EncodedFrame>({
    async transform(frame, controller) {
      try {
        if (operation === 'encrypt') {
          const input = new Uint8Array(frame.data)
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

import { RnnoiseWorkletNode, loadRnnoise } from '@sapphi-red/web-noise-suppressor'
import rnnoiseWorkletPath from '@sapphi-red/web-noise-suppressor/rnnoiseWorklet.js?url'
import rnnoiseWasmPath from '@sapphi-red/web-noise-suppressor/rnnoise.wasm?url'
import rnnoiseWasmSimdPath from '@sapphi-red/web-noise-suppressor/rnnoise_simd.wasm?url'

let cachedWasmBinary: ArrayBuffer | null = null
let workletLoadedOn: AudioContext | null = null

// applyNoiseSuppression returns a new MediaStream whose one audio track is
// rawStream's mic passed through RNNoise, plus a cleanup function that
// must be called when the raw track is replaced or the call ends — an
// AudioContext node graph otherwise leaks (it isn't garbage collected just
// because the MediaStream it feeds stops being used). cleanup also stops
// rawStream's own track(s): the processed track returned in `stream` (from
// ctx.createMediaStreamDestination()) is a distinct MediaStreamTrack object
// from rawStream's raw capture track, so callers that later call .stop() on
// the processed track (setNoiseSuppression's oldTrack.stop(), close()'s
// teardown loop) never reach the raw one — disconnecting the graph nodes
// alone does not stop the underlying capture or release the mic hardware.
export async function applyNoiseSuppression(rawStream: MediaStream, ctx: AudioContext): Promise<{ stream: MediaStream; cleanup: () => void }> {
  if (!cachedWasmBinary) cachedWasmBinary = await loadRnnoise({ url: rnnoiseWasmPath, simdUrl: rnnoiseWasmSimdPath })
  if (workletLoadedOn !== ctx) {
    await ctx.audioWorklet.addModule(rnnoiseWorkletPath)
    workletLoadedOn = ctx
  }
  const source = ctx.createMediaStreamSource(rawStream)
  const node = new RnnoiseWorkletNode(ctx, { wasmBinary: cachedWasmBinary, maxChannels: 1 })
  const dest = ctx.createMediaStreamDestination()
  source.connect(node)
  node.connect(dest)
  return {
    stream: dest.stream,
    cleanup: () => { source.disconnect(); node.disconnect(); rawStream.getTracks().forEach(track => track.stop()) },
  }
}

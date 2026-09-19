type Tone = 'message' | 'member' | 'call'

let context: AudioContext | null = null

function audioContext() {
  if (!context) context = new AudioContext()
  if (context.state === 'suspended') void context.resume()
  return context
}

export function playTone(tone: Tone) {
  try {
    const audio = audioContext()
    const oscillator = audio.createOscillator()
    const gain = audio.createGain()
    const now = audio.currentTime
    oscillator.type = tone === 'call' ? 'sine' : 'triangle'
    oscillator.frequency.setValueAtTime(tone === 'call' ? 740 : tone === 'member' ? 590 : 830, now)
    gain.gain.setValueAtTime(0.0001, now)
    gain.gain.exponentialRampToValueAtTime(0.09, now + 0.015)
    gain.gain.exponentialRampToValueAtTime(0.0001, now + (tone === 'call' ? 0.42 : 0.16))
    oscillator.connect(gain).connect(audio.destination)
    oscillator.start(now)
    oscillator.stop(now + (tone === 'call' ? 0.45 : 0.18))
  } catch { /* browser blocks sound until a user gesture */ }
}

// A call needs a recognisable cadence, not a long music asset that has to be
// licensed, downloaded, and kept in sync with the visual state. This small
// original three-note motif is intentionally sparse: it repeats every 1.8 s,
// leaving enough silence to hear the room around it.
const RING_INTERVAL_MS = 1_800
const ringNotes = [
  { offset: 0, frequency: 659.25 },
  { offset: 0.19, frequency: 783.99 },
  { offset: 0.43, frequency: 659.25 },
]

function scheduleRing(audio: AudioContext, startAt: number) {
  ringNotes.forEach(({ offset, frequency }) => {
    const oscillator = audio.createOscillator()
    const gain = audio.createGain()
    const at = startAt + offset
    oscillator.type = 'sine'
    oscillator.frequency.setValueAtTime(frequency, at)
    gain.gain.setValueAtTime(0.0001, at)
    gain.gain.exponentialRampToValueAtTime(0.065, at + 0.018)
    gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.15)
    oscillator.connect(gain).connect(audio.destination)
    oscillator.start(at)
    oscillator.stop(at + 0.17)
  })
}

export function startIncomingRingtone() {
  try {
    const audio = audioContext()
    const play = () => scheduleRing(audio, audio.currentTime + 0.02)
    play()
    const timer = window.setInterval(play, RING_INTERVAL_MS)
    return () => window.clearInterval(timer)
  } catch {
    return () => undefined
  }
}

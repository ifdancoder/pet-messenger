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

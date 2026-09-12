import type { Session } from '../types'

const key = 'pet-messenger.session.v1'

export function readSession(): Session | null {
  try {
    const value = localStorage.getItem(key)
    return value ? JSON.parse(value) as Session : null
  } catch {
    localStorage.removeItem(key)
    return null
  }
}

export function saveSession(session: Session) { localStorage.setItem(key, JSON.stringify(session)) }

export function clearSession() { localStorage.removeItem(key) }

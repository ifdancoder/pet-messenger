import { clearSession, readSession, saveSession } from './session'
import type { Conversation, MemberProfile, Message, Notification, Session, User } from '../types'

export const apiURL = import.meta.env.VITE_API_URL ?? `http://localhost:${import.meta.env.VITE_PORT ?? '8080'}`

export class APIError extends Error {
  readonly status: number
  constructor(message: string, status: number) { super(message); this.status = status }
}

export type CallJoin = { ticket: string; media_ws_url: string; ice_servers: RTCIceServer[] }
export type Call = { id: string; conversation_id: string; initiator_id: string; status: string; started_at: string }
export type LiveParticipant = { user_id: string; role: string; muted: boolean; self_muted: boolean; track_kinds: string[]; connected: boolean; accepted: boolean }
export type E2EEDevice = { id: string; user_id: string; public_key?: string; fingerprint: string; label: string; status: 'pending' | 'trusted' | 'revoked'; created_at: string }

// Called whenever a 401 triggers a silent token refresh, so the rest of the
// app (which holds its own copy of the session in React state) can pick up
// the new tokens, or log out if the refresh token itself turned out to be
// dead. Registered once by App.
export type SessionRefreshHandler = (session: Session | null) => void
let onSessionRefreshed: SessionRefreshHandler | null = null
export function setSessionRefreshHandler(handler: SessionRefreshHandler | null) { onSessionRefreshed = handler }

async function rawRequest<T>(path: string, token?: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiURL + path, {
    ...init,
    headers: { ...(init?.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }), ...(token ? { Authorization: `Bearer ${token}` } : {}), ...init?.headers },
  })
  const body = await response.json().catch(() => ({})) as { error?: string }
  if (!response.ok) throw new APIError(body.error ?? `Ошибка запроса (${response.status})`, response.status)
  return body as T
}

// Coalesces concurrent refreshes: several requests can hit a 401 for the same
// expired token at once, and only one of them should actually call /auth/refresh.
let refreshInFlight: Promise<string | null> | null = null
function refreshAccessToken(): Promise<string | null> {
  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      const current = readSession()
      if (!current) return null
      try {
        const refreshed = await rawRequest<Session>('/auth/refresh', undefined, { method: 'POST', body: JSON.stringify({ refresh_token: current.refresh_token }) })
        saveSession(refreshed)
        onSessionRefreshed?.(refreshed)
        return refreshed.access_token
      } catch {
        clearSession()
        onSessionRefreshed?.(null)
        return null
      }
    })().finally(() => { refreshInFlight = null })
  }
  return refreshInFlight
}

// request retries once on a 401 after a silent token refresh — every
// authenticated call site below goes through it, so none of them need to
// know refresh tokens exist. /auth/* endpoints skip this: retrying a failed
// login or a failed refresh itself would only loop.
async function request<T>(path: string, token?: string, init?: RequestInit): Promise<T> {
  try {
    return await rawRequest<T>(path, token, init)
  } catch (error) {
    if (error instanceof APIError && error.status === 401 && token && !path.startsWith('/auth/')) {
      const refreshed = await refreshAccessToken()
      if (refreshed) return rawRequest<T>(path, refreshed, init)
    }
    throw error
  }
}

export const api = {
  login: (username: string, password: string) => request<Session>('/auth/login', undefined, { method: 'POST', body: JSON.stringify({ username, password, device_info: 'web' }) }),
  register: (displayName: string, username: string, password: string) => request<Session>('/auth/register', undefined, { method: 'POST', body: JSON.stringify({ display_name: displayName, username, password }) }),
  logout: (refreshToken: string) => request<void>('/auth/logout', undefined, { method: 'POST', body: JSON.stringify({ refresh_token: refreshToken }) }),
  conversations: (token: string) => request<{ conversations: Conversation[] }>('/conversations', token),
  notifications: (token: string) => request<{ notifications: Notification[]; unread_count: number }>('/notifications?limit=100', token),
  messages: (conversationID: string, token: string) => request<{ messages: Message[] }>(`/conversations/${conversationID}/messages`, token),
  userByUsername: (username: string, token: string) => request<User>(`/users/${encodeURIComponent(username)}`, token),
  createDirect: (userID: string, token: string) => request<Conversation>('/conversations/direct', token, { method: 'POST', body: JSON.stringify({ user_id: userID }) }),
  createGroup: (title: string, token: string) => request<Conversation>('/conversations/groups', token, { method: 'POST', body: JSON.stringify({ title }) }),
  members: (conversationID: string, token: string) => request<{ members: MemberProfile[] }>(`/conversations/${conversationID}/members`, token),
  addMember: (conversationID: string, userID: string, token: string) => request<void>(`/conversations/${conversationID}/members`, token, { method: 'POST', body: JSON.stringify({ user_id: userID }) }),
  removeMember: (conversationID: string, userID: string, token: string) => request<void>(`/conversations/${conversationID}/members/${userID}`, token, { method: 'DELETE' }),
  sendMessage: (conversationID: string, content: string, token: string) => request<Message>(`/conversations/${conversationID}/messages`, token, { method: 'POST', body: JSON.stringify({ content, idempotency_key: crypto.randomUUID() }) }),
  markRead: (conversationID: string, messageID: string, token: string) => request<void>(`/conversations/${conversationID}/read`, token, { method: 'POST', body: JSON.stringify({ message_id: messageID }) }),
  startCall: (conversationID: string, token: string) => request<{ call: Call; join: CallJoin }>(`/conversations/${conversationID}/calls`, token, { method: 'POST' }),
  liveCall: (conversationID: string, token: string) => request<{ call: Call; participants: LiveParticipant[] }>(`/conversations/${conversationID}/calls/live`, token),
  joinCall: (callID: string, token: string, role?: 'publisher' | 'viewer') => request<{ join: CallJoin }>(`/calls/${callID}/join`, token, { method: 'POST', body: JSON.stringify({ role: role ?? 'publisher' }) }),
  putCallKeyEnvelopes: (callID: string, token: string, ownerDeviceID: string, envelopes: { recipient_device_id: string; ciphertext: string }[]) => request<void>(`/calls/${callID}/e2ee/key-envelopes`, token, { method: 'PUT', body: JSON.stringify({ owner_device_id: ownerDeviceID, envelopes }) }),
  callKeyEnvelope: (callID: string, token: string, deviceID: string) => request<{ owner_device_id: string; owner_public_key: string; ciphertext: string }>(`/calls/${callID}/e2ee/key-envelope?device_id=${encodeURIComponent(deviceID)}`, token),
  endCall: (callID: string, token: string) => request<void>(`/calls/${callID}/end`, token, { method: 'POST' }),
  muteCallParticipant: (callID: string, userID: string, token: string) => request<void>(`/calls/${callID}/participants/${userID}/mute`, token, { method: 'POST' }),
  unmuteCallParticipant: (callID: string, userID: string, token: string) => request<void>(`/calls/${callID}/participants/${userID}/unmute`, token, { method: 'POST' }),
  removeCallParticipant: (callID: string, userID: string, token: string) => request<void>(`/calls/${callID}/participants/${userID}`, token, { method: 'DELETE' }),
  updateProfile: (patch: { display_name?: string; username?: string }, token: string) => request<User>('/users/me', token, { method: 'PATCH', body: JSON.stringify(patch) }),
  uploadAvatar: (file: File, token: string) => { const form = new FormData(); form.append('avatar', file); return request<User>('/users/me/avatar', token, { method: 'POST', body: form }) },
  registerE2EEDevice: (device: { id: string; public_key: string; signing_public_key: string; label: string }, token: string) => request<E2EEDevice>('/users/me/e2ee/devices', token, { method: 'POST', body: JSON.stringify(device) }),
  e2eeDevices: (token: string) => request<{ devices: E2EEDevice[] }>('/users/me/e2ee/devices', token),
  conversationE2EEDevices: (conversationID: string, token: string) => request<{ devices: E2EEDevice[] }>(`/conversations/${conversationID}/e2ee/devices`, token),
  approveE2EEDevice: (id: string, token: string, approverID: string, signature: string) => request<void>(`/users/me/e2ee/devices/${id}/approve`, token, { method: 'POST', body: JSON.stringify({ approver_id: approverID, signature }) }),
}

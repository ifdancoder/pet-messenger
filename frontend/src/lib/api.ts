import type { Conversation, MemberProfile, Message, Session, User } from '../types'

const apiURL = import.meta.env.VITE_API_URL ?? `http://localhost:${import.meta.env.VITE_PORT ?? '8080'}`

export class APIError extends Error {
  readonly status: number
  constructor(message: string, status: number) { super(message); this.status = status }
}

export type CallJoin = { ticket: string; media_ws_url: string; ice_servers: RTCIceServer[] }
export type Call = { id: string; conversation_id: string; status: string }

async function request<T>(path: string, token?: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiURL + path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}), ...init?.headers },
  })
  const body = await response.json().catch(() => ({})) as { error?: string }
  if (!response.ok) throw new APIError(body.error ?? `Ошибка запроса (${response.status})`, response.status)
  return body as T
}

export const api = {
  login: (username: string, password: string) => request<Session>('/auth/login', undefined, { method: 'POST', body: JSON.stringify({ username, password, device_info: 'web' }) }),
  register: (displayName: string, username: string, password: string) => request<Session>('/auth/register', undefined, { method: 'POST', body: JSON.stringify({ display_name: displayName, username, password }) }),
  conversations: (token: string) => request<{ conversations: Conversation[] }>('/conversations', token),
  messages: (conversationID: string, token: string) => request<{ messages: Message[] }>(`/conversations/${conversationID}/messages`, token),
  userByUsername: (username: string, token: string) => request<User>(`/users/${encodeURIComponent(username)}`, token),
  createDirect: (userID: string, token: string) => request<Conversation>('/conversations/direct', token, { method: 'POST', body: JSON.stringify({ user_id: userID }) }),
  createGroup: (title: string, token: string) => request<Conversation>('/conversations/groups', token, { method: 'POST', body: JSON.stringify({ title }) }),
  members: (conversationID: string, token: string) => request<{ members: MemberProfile[] }>(`/conversations/${conversationID}/members`, token),
  addMember: (conversationID: string, userID: string, token: string) => request<void>(`/conversations/${conversationID}/members`, token, { method: 'POST', body: JSON.stringify({ user_id: userID }) }),
  removeMember: (conversationID: string, userID: string, token: string) => request<void>(`/conversations/${conversationID}/members/${userID}`, token, { method: 'DELETE' }),
  sendMessage: (conversationID: string, content: string, token: string) => request<Message>(`/conversations/${conversationID}/messages`, token, { method: 'POST', body: JSON.stringify({ content, idempotency_key: crypto.randomUUID() }) }),
  startCall: (conversationID: string, token: string) => request<{ call: Call; join: CallJoin }>(`/conversations/${conversationID}/calls`, token, { method: 'POST' }),
  endCall: (callID: string, token: string) => request<void>(`/calls/${callID}/end`, token, { method: 'POST' }),
  muteCallParticipant: (callID: string, userID: string, token: string) => request<void>(`/calls/${callID}/participants/${userID}/mute`, token, { method: 'POST' }),
  unmuteCallParticipant: (callID: string, userID: string, token: string) => request<void>(`/calls/${callID}/participants/${userID}/unmute`, token, { method: 'POST' }),
  removeCallParticipant: (callID: string, userID: string, token: string) => request<void>(`/calls/${callID}/participants/${userID}`, token, { method: 'DELETE' }),
}

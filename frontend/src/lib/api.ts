import type { Conversation, Message, Session } from '../types'

const apiURL = import.meta.env.VITE_API_URL ?? `http://localhost:${import.meta.env.VITE_PORT ?? '8080'}`

export class APIError extends Error {
  readonly status: number
  constructor(message: string, status: number) { super(message); this.status = status }
}

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
  createDirect: (userID: string, token: string) => request<Conversation>('/conversations/direct', token, { method: 'POST', body: JSON.stringify({ user_id: userID }) }),
  createGroup: (title: string, token: string) => request<Conversation>('/conversations/groups', token, { method: 'POST', body: JSON.stringify({ title }) }),
  sendMessage: (conversationID: string, content: string, token: string) => request<Message>(`/conversations/${conversationID}/messages`, token, { method: 'POST', body: JSON.stringify({ content, idempotency_key: crypto.randomUUID() }) }),
}

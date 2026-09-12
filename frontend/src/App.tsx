import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import './App.css'

const api = import.meta.env.VITE_API_URL ?? `http://localhost:${import.meta.env.VITE_PORT ?? '8080'}`
type Session = { access_token: string; user: { id: string; username: string; display_name: string } }
type Conversation = { id: string; kind: string; title: string; last_message_at: string }
type Message = { id: string; sender_id?: string; content: string; created_at: string }

async function request<T>(path: string, token?: string, init?: RequestInit): Promise<T> {
  const response = await fetch(api + path, { ...init, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}), ...init?.headers } })
  const body = await response.json().catch(() => ({}))
  if (response.status === 401 && token) {
    localStorage.removeItem('session')
    window.location.reload()
  }
  if (!response.ok) throw new Error(body.error ?? `Ошибка запроса (${response.status})`)
  return body as T
}

function titleFor(c: Conversation) { return c.title || (c.kind === 'direct' ? 'Личный диалог' : c.kind) }

export default function App() {
  const [session, setSession] = useState<Session | null>(() => JSON.parse(localStorage.getItem('session') ?? 'null'))
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [active, setActive] = useState<Conversation | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [error, setError] = useState('')
  const [registering, setRegistering] = useState(false)
  const [creating, setCreating] = useState(false)

  const loadConversations = useCallback(async () => {
    if (!session) return
    try { setConversations((await request<{ conversations: Conversation[] }>('/conversations', session.access_token)).conversations) }
    catch (e) { setError(e instanceof Error ? e.message : 'Не удалось загрузить диалоги') }
  }, [session])
  const loadMessages = useCallback(async () => {
    if (!active || !session) return
    try { setMessages((await request<{ messages: Message[] }>(`/conversations/${active.id}/messages`, session.access_token)).messages) }
    catch (e) { setError(e instanceof Error ? e.message : 'Не удалось загрузить сообщения') }
  }, [active, session])
  useEffect(() => { void loadConversations() }, [loadConversations])
  useEffect(() => { void loadMessages() }, [loadMessages])
  useEffect(() => {
    if (!session) return
    const timer = window.setInterval(() => { void loadConversations(); void loadMessages() }, 5000)
    return () => window.clearInterval(timer)
  }, [loadConversations, loadMessages, session])

  async function authenticate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError('')
    const form = new FormData(event.currentTarget)
    const path = registering ? '/auth/register' : '/auth/login'
    const payload = registering ? { username: form.get('username'), password: form.get('password'), display_name: form.get('display_name') } : { username: form.get('username'), password: form.get('password'), device_info: 'web' }
    try { const result = await request<Session>(path, undefined, { method: 'POST', body: JSON.stringify(payload) }); localStorage.setItem('session', JSON.stringify(result)); setSession(result) }
    catch (e) { setError(e instanceof Error ? e.message : 'Не удалось войти') }
  }
  async function createConversation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!session) return
    const formElement = event.currentTarget
    const form = new FormData(formElement), kind = String(form.get('kind')), value = String(form.get('value') ?? '').trim()
    if (!value) return
    try {
      const c = await request<Conversation>(kind === 'direct' ? '/conversations/direct' : '/conversations/groups', session.access_token, { method: 'POST', body: JSON.stringify(kind === 'direct' ? { user_id: value } : { title: value }) })
      setConversations(previous => [c, ...previous.filter(item => item.id !== c.id)]); setActive(c); setMessages([]); setCreating(false); formElement.reset()
    } catch (e) { setError(e instanceof Error ? e.message : 'Не удалось создать диалог') }
  }
  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!active || !session) return
    const formElement = event.currentTarget
    const content = String(new FormData(formElement).get('content') ?? '').trim(); if (!content) return
    try { const m = await request<Message>(`/conversations/${active.id}/messages`, session.access_token, { method: 'POST', body: JSON.stringify({ content, idempotency_key: crypto.randomUUID() }) }); setMessages(previous => [...previous, m]); formElement.reset(); void loadConversations() }
    catch (e) { setError(e instanceof Error ? e.message : 'Не удалось отправить сообщение') }
  }
  async function copyMyID() {
    if (!session) return
    try { await navigator.clipboard.writeText(session.user.id) }
    catch { setError('Не удалось скопировать UUID. Скопируйте его из строки ниже.') }
  }
  function logout() { localStorage.removeItem('session'); setSession(null); setActive(null); setMessages([]); setError('') }

  if (!session) return <main className="auth"><section><p className="brand">pet messenger</p><h1>{registering ? 'Создать аккаунт' : 'Войти'}</h1><form onSubmit={authenticate}>{registering && <input name="display_name" placeholder="Имя" required />}<input name="username" placeholder="Логин" required autoComplete="username" /><input name="password" type="password" placeholder="Пароль (минимум 8 символов)" required autoComplete={registering ? 'new-password' : 'current-password'} /><button>{registering ? 'Зарегистрироваться' : 'Войти'}</button></form>{error && <p className="error">{error}</p>}<button className="secondary" onClick={() => { setRegistering(value => !value); setError('') }}>{registering ? 'У меня есть аккаунт' : 'Создать аккаунт'}</button></section></main>
  return <main className="app"><aside><header><strong>pet messenger</strong><button className="secondary compact" onClick={logout}>Выйти</button></header><div className="profile">{session.user.display_name || session.user.username}<small>@{session.user.username}</small><code>{session.user.id}</code><button className="secondary compact" onClick={() => void copyMyID()}>Копировать мой UUID</button></div><div className="sidebar-title"><h2>Диалоги</h2><button className="compact" onClick={() => setCreating(value => !value)}>Новый</button></div>{creating && <form className="new-conversation" onSubmit={createConversation}><select name="kind"><option value="direct">Личный</option><option value="group">Группа</option></select><input name="value" placeholder="UUID пользователя или название" required /><button>Создать</button></form>}<nav>{conversations.map(c => <button className={active?.id === c.id ? 'conversation active' : 'conversation'} key={c.id} onClick={() => { setActive(c); setError('') }}><b>{titleFor(c)}</b><small>{c.kind === 'direct' ? 'личный' : c.kind}</small></button>)}</nav></aside><section className="chat"><header><h1>{active ? titleFor(active) : 'Выберите диалог'}</h1></header>{error && <p className="error chat-error">{error}</p>}<div className="messages">{active && messages.length === 0 && <p className="empty">Сообщений пока нет.</p>}{!active && <p className="empty">Создайте или выберите диалог слева.</p>}{messages.map(m => <article className={m.sender_id === session.user.id ? 'mine' : ''} key={m.id}><p>{m.content || 'Системное сообщение'}</p><small>{new Date(m.created_at).toLocaleTimeString()}</small></article>)}</div>{active && <form className="composer" onSubmit={send}><input name="content" placeholder="Сообщение" autoComplete="off" /><button>Отправить</button></form>}</section></main>
}

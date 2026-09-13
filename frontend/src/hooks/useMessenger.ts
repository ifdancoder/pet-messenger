import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, api } from '../lib/api'
import { clearSession } from '../lib/session'
import { playTone } from '../lib/sounds'
import type { Conversation, Message, Session } from '../types'
import { useRealtimeChat } from './useRealtimeChat'

const refreshInterval = 5_000

function chronological(messages: Message[]) {
  return [...new Map(messages.map(message => [message.id, message])).values()].sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0) || a.created_at.localeCompare(b.created_at))
}

export function useMessenger(session: Session, onSessionExpired: () => void) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeConversation, setActiveConversation] = useState<Conversation | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [error, setError] = useState('')
  const [isLoadingConversations, setIsLoadingConversations] = useState(true)
  const [isLoadingMessages, setIsLoadingMessages] = useState(false)
  const [unreadByConversation, setUnreadByConversation] = useState<Record<string, number>>({})
  const messageRequestID = useRef(0)
  const knownNotifications = useRef<Set<string> | null>(null)
  const markedRead = useRef<Record<string, string>>({})

  const handleError = useCallback((error: unknown) => {
    if (error instanceof APIError && error.status === 401) {
      clearSession(); onSessionExpired(); return
    }
    setError(error instanceof Error ? error.message : 'Не удалось выполнить действие')
  }, [onSessionExpired])

  const loadConversations = useCallback(async () => {
    try { setConversations((await api.conversations(session.access_token)).conversations) }
    catch (error) { handleError(error) }
    finally { setIsLoadingConversations(false) }
  }, [handleError, session.access_token])

  const loadNotifications = useCallback(async () => {
    try {
      const result = await api.notifications(session.access_token)
      const unread: Record<string, number> = {}
      for (const notification of result.notifications) {
        const conversationID = notification.payload?.conversation_id
        if (!conversationID || notification.read_at || conversationID === activeConversation?.id) continue
        unread[conversationID] = (unread[conversationID] ?? 0) + 1
      }
      const previous = knownNotifications.current
      if (previous) for (const notification of result.notifications) {
        if (!previous.has(notification.id) && !notification.read_at) playTone(notification.kind === 'call_started' ? 'call' : 'message')
      }
      knownNotifications.current = new Set(result.notifications.map(notification => notification.id))
      setUnreadByConversation(unread)
    } catch (error) { handleError(error) }
  }, [activeConversation, handleError, session.access_token])

  const loadMessages = useCallback(async (conversation: Conversation) => {
    const requestID = ++messageRequestID.current
    setIsLoadingMessages(true)
    try {
      const result = await api.messages(conversation.id, session.access_token)
      if (requestID === messageRequestID.current) {
        const ordered = chronological(result.messages)
        setMessages(ordered)
        const latest = ordered.at(-1)
        if (latest && markedRead.current[conversation.id] !== latest.id) {
          markedRead.current[conversation.id] = latest.id
          void api.markRead(conversation.id, latest.id, session.access_token).catch(handleError)
        }
      }
    }
    catch (error) { handleError(error) }
    finally { if (requestID === messageRequestID.current) setIsLoadingMessages(false) }
  }, [handleError, session.access_token])

  useEffect(() => { void Promise.resolve().then(loadConversations); void Promise.resolve().then(loadNotifications) }, [loadConversations, loadNotifications])
  useEffect(() => { if (activeConversation) void Promise.resolve().then(() => loadMessages(activeConversation)) }, [activeConversation, loadMessages])
  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadConversations()
      void loadNotifications()
      if (activeConversation) void loadMessages(activeConversation)
    }, refreshInterval)
    return () => window.clearInterval(timer)
  }, [activeConversation, loadConversations, loadMessages, loadNotifications])

  const selectConversation = useCallback((conversation: Conversation) => { setError(''); setUnreadByConversation(previous => ({ ...previous, [conversation.id]: 0 })); setActiveConversation(conversation) }, [])
  const createConversation = useCallback(async (kind: 'direct' | 'group', value: string) => {
    try {
      const conversation = kind === 'direct'
        ? await api.userByUsername(value, session.access_token).then(async user => ({ ...(await api.createDirect(user.id, session.access_token)), direct_username: user.username, direct_display_name: user.display_name }))
        : await api.createGroup(value, session.access_token)
      setConversations(previous => [conversation, ...previous.filter(item => item.id !== conversation.id)])
      setActiveConversation(conversation); setMessages([]); setError('')
      return true
    } catch (error) { handleError(error); return false }
  }, [handleError, session.access_token])
  const sendMessage = useCallback(async (content: string) => {
    if (!activeConversation) return false
    try {
      const message = await api.sendMessage(activeConversation.id, content, session.access_token)
      setMessages(previous => chronological([...previous, message])); void loadConversations(); setError('')
      return true
    } catch (error) { handleError(error); return false }
  }, [activeConversation, handleError, loadConversations, session.access_token])

  const receiveRealtimeFrame = useCallback((frame: { type: string; conversation_id?: string; message?: Message }) => {
    if (!frame.conversation_id || frame.conversation_id !== activeConversation?.id) return
    if (frame.type === 'message.created' && frame.message) {
      const message = frame.message
      if (message.sender_id !== session.user.id) {
        setMessages(previous => chronological([...previous, message]))
        playTone(message.type === 'system' ? 'member' : 'message')
      }
      void loadConversations()
    } else if (frame.type === 'message.updated' || frame.type === 'message.deleted') void loadMessages(activeConversation)
  }, [activeConversation, loadConversations, loadMessages, session.user.id])
  const isRealtimeConnected = useRealtimeChat(session.access_token, activeConversation?.id, receiveRealtimeFrame)

  return { conversations, activeConversation, messages, unreadByConversation, error, setError, isLoadingConversations, isLoadingMessages, isRealtimeConnected, selectConversation, createConversation, sendMessage }
}

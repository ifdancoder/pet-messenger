import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, api } from '../lib/api'
import { clearSession } from '../lib/session'
import type { Conversation, Message, Session } from '../types'
import { useRealtimeChat } from './useRealtimeChat'

const refreshInterval = 5_000

export function useMessenger(session: Session, onSessionExpired: () => void) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeConversation, setActiveConversation] = useState<Conversation | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [error, setError] = useState('')
  const [isLoadingConversations, setIsLoadingConversations] = useState(true)
  const [isLoadingMessages, setIsLoadingMessages] = useState(false)
  const messageRequestID = useRef(0)

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

  const loadMessages = useCallback(async (conversation: Conversation) => {
    const requestID = ++messageRequestID.current
    setIsLoadingMessages(true)
    try {
      const result = await api.messages(conversation.id, session.access_token)
      if (requestID === messageRequestID.current) setMessages(result.messages)
    }
    catch (error) { handleError(error) }
    finally { if (requestID === messageRequestID.current) setIsLoadingMessages(false) }
  }, [handleError, session.access_token])

  useEffect(() => { void Promise.resolve().then(loadConversations) }, [loadConversations])
  useEffect(() => { if (activeConversation) void Promise.resolve().then(() => loadMessages(activeConversation)) }, [activeConversation, loadMessages])
  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadConversations()
      if (activeConversation) void loadMessages(activeConversation)
    }, refreshInterval)
    return () => window.clearInterval(timer)
  }, [activeConversation, loadConversations, loadMessages])

  const selectConversation = useCallback((conversation: Conversation) => { setError(''); setActiveConversation(conversation) }, [])
  const createConversation = useCallback(async (kind: 'direct' | 'group', value: string) => {
    try {
      const conversation = kind === 'direct'
        ? await api.userByUsername(value, session.access_token).then(user => api.createDirect(user.id, session.access_token))
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
      setMessages(previous => [...previous, message]); void loadConversations(); setError('')
      return true
    } catch (error) { handleError(error); return false }
  }, [activeConversation, handleError, loadConversations, session.access_token])

  const receiveRealtimeFrame = useCallback((frame: { type: string; conversation_id?: string; message?: Message }) => {
    if (!frame.conversation_id || frame.conversation_id !== activeConversation?.id) return
    if (frame.type === 'message.created' && frame.message) {
      const message = frame.message
      setMessages(previous => previous.some(item => item.id === message.id) ? previous : [...previous, message])
      void loadConversations()
    } else if (frame.type === 'message.updated' || frame.type === 'message.deleted') void loadMessages(activeConversation)
  }, [activeConversation, loadConversations, loadMessages])
  const isRealtimeConnected = useRealtimeChat(session.access_token, activeConversation?.id, receiveRealtimeFrame)

  return { conversations, activeConversation, messages, error, setError, isLoadingConversations, isLoadingMessages, isRealtimeConnected, selectConversation, createConversation, sendMessage }
}

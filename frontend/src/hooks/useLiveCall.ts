import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, api } from '../lib/api'
import type { Call } from '../lib/api'
import type { Conversation, Session } from '../types'

const refreshInterval = 2_500

export function useLiveCall(session: Session, conversation: Conversation | null) {
  const [liveCall, setLiveCall] = useState<Call | null>(null)
  const requestID = useRef(0)
  const refresh = useCallback(async () => {
    if (!conversation) return
    const id = ++requestID.current
    try {
      const { call } = await api.liveCall(conversation.id, session.access_token)
      if (id === requestID.current) setLiveCall(call)
    } catch (error) {
      if (id === requestID.current && error instanceof APIError && error.status === 404) setLiveCall(null)
    }
  }, [conversation, session.access_token])
  useEffect(() => {
    const timer = window.setInterval(() => void refresh(), refreshInterval)
    return () => window.clearInterval(timer)
  }, [refresh])
  return liveCall?.conversation_id === conversation?.id ? liveCall : null
}

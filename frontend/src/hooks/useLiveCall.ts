import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, api } from '../lib/api'
import type { Call, LiveParticipant } from '../lib/api'
import type { Conversation, Session } from '../types'

const refreshInterval = 2_500
export type LiveCall = { call: Call; conversation: Conversation; participants: LiveParticipant[] }

export function useLiveCalls(session: Session, conversations: Conversation[]) {
  const [calls, setCalls] = useState<LiveCall[]>([])
  const requestID = useRef(0)
  const refresh = useCallback(async () => {
    const id = ++requestID.current
    const results = await Promise.all(conversations.map(async conversation => {
      try { const { call, participants } = await api.liveCall(conversation.id, session.access_token); return { call, conversation, participants } }
      catch (error) { if (error instanceof APIError && error.status === 404) return null; throw error }
    }))
    if (id === requestID.current) setCalls(results.filter((item): item is LiveCall => item !== null))
  }, [conversations, session.access_token])
  useEffect(() => { void refresh(); const timer = window.setInterval(() => void refresh(), refreshInterval); return () => window.clearInterval(timer) }, [refresh])
  return calls
}

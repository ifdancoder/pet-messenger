import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, api } from '../lib/api'
import type { Call, LiveParticipant } from '../lib/api'
import type { Conversation, Session } from '../types'

const refreshInterval = 2_500
export type LiveCall = { call: Call; conversation: Conversation; participants: LiveParticipant[] }

// skip: while true, no fetch is ever made (not even a first one against a
// not-yet-real conversations list) - callers with an as-yet-unsettled
// conversations list (e.g. still loading) should pass skip=true so the
// hook's first real fetch, once skip flips false, is unambiguously "the
// first snapshot fetched against the real list", not a placeholder pass a
// caller would otherwise have to guess about via tick-counting.
export function useLiveCalls(session: Session, conversations: Conversation[], skip: boolean) {
  const [calls, setCalls] = useState<LiveCall[]>([])
  const [hasLoadedOnce, setHasLoadedOnce] = useState(false)
  const requestID = useRef(0)
  const refresh = useCallback(async () => {
    if (skip) return
    const id = ++requestID.current
    const results = await Promise.all(conversations.map(async conversation => {
      try { const { call, participants } = await api.liveCall(conversation.id, session.access_token); return { call, conversation, participants } }
      catch (error) { if (error instanceof APIError && error.status === 404) return null; throw error }
    }))
    if (id === requestID.current) { setCalls(results.filter((item): item is LiveCall => item !== null)); setHasLoadedOnce(true) }
  }, [conversations, session.access_token, skip])
  useEffect(() => {
    if (skip) return
    void refresh()
    const timer = window.setInterval(() => void refresh(), refreshInterval)
    return () => window.clearInterval(timer)
  }, [refresh, skip])
  return { calls, hasLoadedOnce }
}

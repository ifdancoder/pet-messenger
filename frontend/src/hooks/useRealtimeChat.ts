import { useEffect, useRef, useState } from 'react'
import type { Message } from '../types'

type Frame = { type: string; conversation_id?: string; message?: Message }

function websocketURL() {
  const apiURL = import.meta.env.VITE_API_URL ?? `http://localhost:${import.meta.env.VITE_PORT ?? '8080'}`
  return `${apiURL.replace(/^http/, 'ws')}/ws`
}

export function useRealtimeChat(token: string, conversationID: string | undefined, onMessage: (frame: Frame) => void) {
  const socket = useRef<WebSocket | null>(null)
  const conversationRef = useRef(conversationID)
  const messageHandler = useRef(onMessage)
  const reconnectAttempt = useRef(0)
  const [isConnected, setIsConnected] = useState(false)
  useEffect(() => { conversationRef.current = conversationID }, [conversationID])
  useEffect(() => { messageHandler.current = onMessage }, [onMessage])
  useEffect(() => {
    let closedByClient = false
    let reconnectTimer: number | undefined
    function subscribe() {
      const active = conversationRef.current
      if (active && socket.current?.readyState === WebSocket.OPEN) socket.current.send(JSON.stringify({ type: 'subscribe', conversation_id: active }))
    }
    function connect() {
      const current = new WebSocket(websocketURL(), ['bearer', token])
      socket.current = current
      current.onopen = () => { reconnectAttempt.current = 0; setIsConnected(true); subscribe() }
      current.onmessage = event => { try { messageHandler.current(JSON.parse(String(event.data)) as Frame) } catch { /* malformed frame */ } }
      current.onclose = () => {
        setIsConnected(false)
        if (closedByClient) return
        reconnectTimer = window.setTimeout(connect, Math.min(1_000 * 2 ** reconnectAttempt.current++, 15_000))
      }
      current.onerror = () => current.close()
    }
    connect()
    return () => { closedByClient = true; if (reconnectTimer) window.clearTimeout(reconnectTimer); socket.current?.close(); socket.current = null }
  }, [token])
  useEffect(() => { if (conversationID && socket.current?.readyState === WebSocket.OPEN) socket.current.send(JSON.stringify({ type: 'subscribe', conversation_id: conversationID })) }, [conversationID])
  return isConnected
}

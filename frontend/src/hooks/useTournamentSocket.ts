import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getToken } from '../api/client'

const INITIAL_RECONNECT_DELAY_MS = 1000
const MAX_RECONNECT_DELAY_MS = 30000
const RECONNECT_FAILURES_BEFORE_BANNER = 3

export function useTournamentSocket(tournamentId: string | undefined): { connectionLost: boolean } {
  const queryClient = useQueryClient()
  const [connectionLost, setConnectionLost] = useState(false)

  const failureCountRef = useRef(0)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (!tournamentId) return

    failureCountRef.current = 0

    let socket: WebSocket | null = null
    let cancelled = false

    function connect() {
      if (cancelled) return
      const token = getToken()
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(`${protocol}//${window.location.host}/api/tournaments/${tournamentId}/ws?token=${token}`)

      socket.onopen = () => {
        failureCountRef.current = 0
        setConnectionLost(false)
        void queryClient.invalidateQueries({ queryKey: ['standings', tournamentId] })
      }
      socket.onmessage = () => {
        void queryClient.invalidateQueries({ queryKey: ['standings', tournamentId] })
      }
      socket.onclose = () => {
        if (cancelled) return
        failureCountRef.current += 1
        if (failureCountRef.current >= RECONNECT_FAILURES_BEFORE_BANNER) {
          setConnectionLost(true)
        }
        const delay = Math.min(
          INITIAL_RECONNECT_DELAY_MS * 2 ** (failureCountRef.current - 1),
          MAX_RECONNECT_DELAY_MS,
        )
        reconnectTimeoutRef.current = setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      cancelled = true
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
      socket?.close()
    }
  }, [tournamentId, queryClient])

  return { connectionLost }
}

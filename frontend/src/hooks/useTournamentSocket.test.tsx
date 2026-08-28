import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTournamentSocket } from './useTournamentSocket'

vi.mock('../api/client', () => ({ getToken: () => 'test-token' }))

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: (() => void) | null = null
  onclose: (() => void) | null = null
  url: string
  closed = false

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  close() {
    this.closed = true
  }
}

describe('useTournamentSocket', () => {
  let queryClient: QueryClient
  let originalWebSocket: typeof WebSocket

  beforeEach(() => {
    vi.useFakeTimers()
    FakeWebSocket.instances = []
    originalWebSocket = globalThis.WebSocket
    // @ts-expect-error -- test double, not a full WebSocket implementation
    globalThis.WebSocket = FakeWebSocket
    queryClient = new QueryClient()
    vi.spyOn(queryClient, 'invalidateQueries')
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    vi.useRealTimers()
  })

  function wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('opens a socket to the tournament endpoint with the auth token', () => {
    renderHook(() => useTournamentSocket('42'), { wrapper })

    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].url).toContain('/api/tournaments/42/ws?token=test-token')
  })

  it('invalidates the standings query on any message', () => {
    renderHook(() => useTournamentSocket('42'), { wrapper })

    FakeWebSocket.instances[0].onmessage?.()

    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['standings', '42'] })
  })

  it('reconnects with backoff and sets connectionLost after repeated failures', async () => {
    const { result } = renderHook(() => useTournamentSocket('42'), { wrapper })

    expect(result.current.connectionLost).toBe(false)

    for (let i = 0; i < 3; i++) {
      const current = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
      current.onclose?.()
      await vi.runOnlyPendingTimersAsync()
    }

    expect(result.current.connectionLost).toBe(true)
    expect(FakeWebSocket.instances.length).toBeGreaterThan(1)
  })

  it('resets the failure count when tournamentId changes while the hook stays mounted', async () => {
    const { result, rerender } = renderHook(
      ({ tournamentId }: { tournamentId: string }) => useTournamentSocket(tournamentId),
      { wrapper, initialProps: { tournamentId: '1' } },
    )

    // Drive 2 failed connection attempts on tournament '1' -- below the 3-failure threshold,
    // so connectionLost stays false but the internal failure count sits at 2.
    for (let i = 0; i < 2; i++) {
      const current = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
      current.onclose?.()
      await vi.runOnlyPendingTimersAsync()
    }
    expect(result.current.connectionLost).toBe(false)

    // Simulate a route param change that keeps the component mounted (e.g. navigating
    // from one tournament's Watch page to another's).
    rerender({ tournamentId: '2' })

    const newSocket = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
    expect(newSocket.url).toContain('/api/tournaments/2/ws')

    // A single failure on the NEW connection must not immediately flip connectionLost --
    // the failure count must have been reset to 0, not carried over from tournament '1'.
    newSocket.onclose?.()
    await vi.runOnlyPendingTimersAsync()

    expect(result.current.connectionLost).toBe(false)
  })

  it('closes the socket on unmount', () => {
    const { unmount } = renderHook(() => useTournamentSocket('42'), { wrapper })
    const socket = FakeWebSocket.instances[0]

    unmount()

    expect(socket.closed).toBe(true)
  })
})

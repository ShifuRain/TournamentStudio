import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setToken, setUnauthorizedHandler } from './client'

describe('api client', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('attaches the bearer token when one is stored', async () => {
    setToken('abc123')
    ;(fetch as any).mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }))

    await api.get('/api/whoami')

    const [, init] = (fetch as any).mock.calls[0]
    expect(init.headers.get('Authorization')).toBe('Bearer abc123')
  })

  it('clears the token and calls the unauthorized handler on 401', async () => {
    setToken('abc123')
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    ;(fetch as any).mockResolvedValue(new Response('unauthorized', { status: 401 }))

    await expect(api.get('/api/whoami')).rejects.toBeInstanceOf(ApiError)
    expect(localStorage.getItem('ts_token')).toBeNull()
    expect(handler).toHaveBeenCalled()
  })

  it('throws ApiError with the response body text on other errors', async () => {
    ;(fetch as any).mockResolvedValue(new Response('bad request', { status: 400 }))

    await expect(api.get('/api/whoami')).rejects.toMatchObject({ status: 400, message: 'bad request' })
  })

  it('sends a JSON body with Content-Type on post', async () => {
    ;(fetch as any).mockResolvedValue(new Response(JSON.stringify({ id: 1 }), { status: 201 }))

    await api.post('/api/tournaments', { name: 'Test' })

    const [, init] = (fetch as any).mock.calls[0]
    expect(init.method).toBe('POST')
    expect(init.headers.get('Content-Type')).toBe('application/json')
    expect(init.body).toBe(JSON.stringify({ name: 'Test' }))
  })
})

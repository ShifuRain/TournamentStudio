import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider, useAuth } from './AuthContext'
import * as client from '../api/client'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return {
    ...actual,
    api: { post: vi.fn(), get: vi.fn(), patch: vi.fn(), postForm: vi.fn() },
  }
})

function Consumer() {
  const { token, role, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="token">{token ?? 'none'}</span>
      <span data-testid="role">{role ?? 'none'}</span>
      <button onClick={() => login('organizer1', 'pw')}>login</button>
      <button onClick={() => logout()}>logout</button>
    </div>
  )
}

describe('AuthContext', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.mocked(client.api.post).mockReset()
  })

  it('login stores token and role', async () => {
    vi.mocked(client.api.post).mockResolvedValue({ token: 'abc', role: 'organizer' })
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )

    await userEvent.click(screen.getByText('login'))

    await waitFor(() => expect(screen.getByTestId('token')).toHaveTextContent('abc'))
    expect(screen.getByTestId('role')).toHaveTextContent('organizer')
    expect(localStorage.getItem('ts_token')).toBe('abc')
  })

  it('logout clears token and role', async () => {
    vi.mocked(client.api.post).mockResolvedValue({ token: 'abc', role: 'organizer' })
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )
    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('token')).toHaveTextContent('abc'))

    vi.mocked(client.api.post).mockResolvedValue(undefined)
    await userEvent.click(screen.getByText('logout'))

    await waitFor(() => expect(screen.getByTestId('token')).toHaveTextContent('none'))
    expect(localStorage.getItem('ts_token')).toBeNull()
  })
})

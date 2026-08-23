import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { LoginPage } from './LoginPage'
import { ApiError } from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const loginMock = vi.fn()
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ login: loginMock, logout: vi.fn(), token: null, role: null }),
}))

describe('LoginPage', () => {
  it('submits username and password', async () => {
    loginMock.mockResolvedValue(undefined)
    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )

    await userEvent.type(screen.getByLabelText('login_username'), 'organizer1')
    await userEvent.type(screen.getByLabelText('login_password'), 'secret')
    await userEvent.click(screen.getByText('login_submit'))

    expect(loginMock).toHaveBeenCalledWith('organizer1', 'secret')
  })

  it('shows an error message on invalid credentials', async () => {
    loginMock.mockRejectedValue(new ApiError(401, 'invalid credentials'))
    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )

    await userEvent.type(screen.getByLabelText('login_username'), 'organizer1')
    await userEvent.type(screen.getByLabelText('login_password'), 'wrong')
    await userEvent.click(screen.getByText('login_submit'))

    expect(await screen.findByRole('alert')).toHaveTextContent('login_invalid_credentials')
  })
})

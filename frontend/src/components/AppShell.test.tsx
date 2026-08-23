import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AppShell } from './AppShell'

const logoutMock = vi.fn()

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}))
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ role: 'organizer', token: 'abc', logout: logoutMock }),
}))
vi.mock('../i18n/i18n', () => ({
  AVAILABLE_LANGUAGES: ['en', 'de'],
  changeLanguage: vi.fn(),
}))

function renderShell() {
  render(
    <MemoryRouter initialEntries={['/inner']}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/inner" element={<div>inner content</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  )
}

describe('AppShell', () => {
  it('renders the current role and the routed child content', () => {
    renderShell()
    expect(screen.getByText('organizer')).toBeInTheDocument()
    expect(screen.getByText('inner content')).toBeInTheDocument()
  })

  it('calls logout when the logout button is clicked', async () => {
    renderShell()
    await userEvent.click(screen.getByText('nav_logout'))
    expect(logoutMock).toHaveBeenCalled()
  })
})

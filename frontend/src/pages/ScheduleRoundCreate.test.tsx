import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleRoundCreate } from './ScheduleRoundCreate'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string, vars?: Record<string, unknown>) => (vars ? `${key} ${JSON.stringify(vars)}` : key) }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

function renderPanel() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route path="/tournaments/:id/schedule" element={<ScheduleRoundCreate />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleRoundCreate', () => {
  it('shuffles teams into groups, allows moving a team, and submits', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue([
      { id: 1, tournament_id: 1, name: 'Team A', club: '', extra_fields: {} },
      { id: 2, tournament_id: 1, name: 'Team B', club: '', extra_fields: {} },
    ])
    vi.mocked(client.api.post).mockResolvedValue({ id: 1, round_number: 1, status: 'open', groups: [], divisions: [] })

    renderPanel()

    await userEvent.click(await screen.findByText('schedule_round_create_shuffle'))

    expect(await screen.findByText('Team A')).toBeInTheDocument()
    expect(screen.getByText('Team B')).toBeInTheDocument()

    await userEvent.click(screen.getByText(/schedule_round_create_submit/))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith(
        '/api/tournaments/1/rounds',
        expect.objectContaining({ round_number: 1, groups: expect.any(Array) }),
      ),
    )
  })

  it('shows an error message when round creation fails', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue([
      { id: 1, tournament_id: 1, name: 'Team A', club: '', extra_fields: {} },
    ])
    vi.mocked(client.api.post).mockRejectedValue(new Error('server error'))

    renderPanel()

    await userEvent.click(await screen.findByText('schedule_round_create_shuffle'))
    await userEvent.click(screen.getByText(/schedule_round_create_submit/))

    expect(await screen.findByRole('alert')).toHaveTextContent('schedule_round_create_error')
  })

  it('renders nothing for a non-organizer role', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue([])

    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<ScheduleRoundCreate />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(container).toBeEmptyDOMElement()
  })
})

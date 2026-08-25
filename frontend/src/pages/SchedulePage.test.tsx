import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SchedulePage } from './SchedulePage'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, vars?: Record<string, unknown>) => (vars ? `${key} ${JSON.stringify(vars)}` : key),
  }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

describe('SchedulePage', () => {
  it('renders the Courses section', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds') return Promise.resolve({ rounds: [] })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      if (p === '/api/tournaments/1/teams') return Promise.resolve([])
      if (p === '/api/tournaments/1/schedule') return Promise.resolve({ heats: [] })
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_courses_title')).toBeInTheDocument()
  })

  it('renders the round-create panel only when no round exists yet', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds') return Promise.resolve({ rounds: [] })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      if (p === '/api/tournaments/1/teams') return Promise.resolve([])
      if (p === '/api/tournaments/1/schedule') return Promise.resolve({ heats: [] })
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_round_create_title')).toBeInTheDocument()
  })

  it('hides the round-create panel once a round exists', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds')
        return Promise.resolve({ rounds: [{ id: 1, round_number: 1, status: 'open', groups: [], divisions: [] }] })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      if (p === '/api/tournaments/1/teams') return Promise.resolve([])
      if (p === '/api/tournaments/1/schedule') return Promise.resolve({ heats: [] })
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_courses_title')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('schedule_round_create_title')).not.toBeInTheDocument())
  })

  it('renders the group-scheduling panel with ordinal (not raw id) group labels', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1/rounds')
        return Promise.resolve({
          rounds: [
            {
              id: 1,
              round_number: 2,
              status: 'open',
              // Global autoincrement IDs, as seen live (round 2's groups came
              // back with id 5 and 6, not 1 and 2) -- the label must not
              // leak these raw ids.
              groups: [
                { id: 5, team_ids: ['1', '2'] },
                { id: 6, team_ids: ['3'] },
              ],
              divisions: [],
            },
          ],
        })
      if (p === '/api/tournaments/1/courses') return Promise.resolve({ courses: [] })
      if (p === '/api/tournaments/1/teams') return Promise.resolve([])
      if (p === '/api/tournaments/1/schedule') return Promise.resolve({ heats: [] })
      return Promise.reject(new Error(`unexpected path ${p}`))
    })

    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route path="/tournaments/:id/schedule" element={<SchedulePage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('schedule_assignments_group_title')).toBeInTheDocument()
    expect(screen.getByText('schedule_round_create_group_label {"number":1} (2 teams)')).toBeInTheDocument()
    expect(screen.getByText('schedule_round_create_group_label {"number":2} (1 teams)')).toBeInTheDocument()
    expect(screen.queryByText(/Group 5/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Group 6/)).not.toBeInTheDocument()
  })
})

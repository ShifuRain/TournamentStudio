import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleHeats } from './ScheduleHeats'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course, Heat, Round, Team } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, post: vi.fn(), patch: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

const round: Round = {
  id: 1,
  round_number: 1,
  status: 'open',
  groups: [{ id: 10, team_ids: ['1', '2'] }],
  divisions: [],
}
const courses: Course[] = [
  { id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 },
]
const teams: Team[] = [
  { id: 1, tournament_id: 1, name: 'Team One', club: 'Club A', extra_fields: {} },
  { id: 2, tournament_id: 1, name: 'Team Two', club: 'Club B', extra_fields: {} },
]
const openHeat: Heat = {
  id: 100,
  round_id: 1,
  group_id: 10,
  division_id: null,
  course_id: 1,
  planned_start: '2026-01-01T10:00:00Z',
  effective_start: '2026-01-01T10:00:00Z',
  status: 'scheduled',
  results: [],
}

function renderHeats(heats: Heat[]) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route
            path="/tournaments/:id/schedule"
            element={<ScheduleHeats heats={heats} courses={courses} currentRound={round} teams={teams} />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleHeats', () => {
  it('submits results for both teams in an open heat', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.post).mockResolvedValue({ results_recorded: 2 })

    renderHeats([openHeat])

    await userEvent.type(screen.getByLabelText('schedule_heats_results_time — 1'), '100.5')
    await userEvent.type(screen.getByLabelText('schedule_heats_results_time — 2'), '105.2')
    await userEvent.click(screen.getByText('schedule_heats_results_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/heats/100/results', {
        '1': { time_seconds: 100.5 },
        '2': { time_seconds: 105.2 },
      }),
    )
  })

  it('does not render a results form for a closed heat', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderHeats([{ ...openHeat, status: 'closed' }])

    expect(screen.queryByText('schedule_heats_results_submit')).not.toBeInTheDocument()
  })

  it('hides results entry for a spectator', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'spectator', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderHeats([openHeat])

    expect(screen.queryByText('schedule_heats_results_submit')).not.toBeInTheDocument()
  })

  it('renders nothing when there are no heats', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    const { container } = renderHeats([])
    expect(container).toBeEmptyDOMElement()
  })

  it('shows team names instead of raw team IDs in the results table', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderHeats([openHeat])

    expect(screen.getByText('Team One')).toBeInTheDocument()
    expect(screen.getByText('Team Two')).toBeInTheDocument()
    expect(screen.queryByText('1', { selector: 'td' })).not.toBeInTheDocument()
    expect(screen.queryByText('2', { selector: 'td' })).not.toBeInTheDocument()
  })

  it('shows the effective start time in the heat row for every role', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'spectator', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderHeats([openHeat])

    const expected = new Date(openHeat.effective_start).toLocaleString()
    expect(
      screen.getByText((_, node) => {
        if (!node?.textContent?.includes(expected)) return false
        return Array.from(node.children).every((child) => !child.textContent?.includes(expected))
      }),
    ).toBeInTheDocument()
  })
})

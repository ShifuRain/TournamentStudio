import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WatchPage } from './WatchPage'
import * as client from '../api/client'
import type { StandingsResponse, Team } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string, opts?: Record<string, unknown>) => (opts ? `${key} ${JSON.stringify(opts)}` : key) }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})
vi.mock('../hooks/useTournamentSocket', () => ({
  useTournamentSocket: () => ({ connectionLost: false }),
}))

const standings: StandingsResponse = {
  rounds: [
    {
      id: 1,
      round_number: 1,
      status: 'open',
      standings: [
        {
          group_id: 10,
          division_id: null,
          division_name: null,
          ranked_teams: [
            { rank: 1, team_id: '1', time_seconds: 100.5, status: '' },
            { rank: 2, team_id: '2', time_seconds: null, status: 'DNF' },
          ],
        },
      ],
    },
  ],
}
const teams: Team[] = [
  { id: 1, tournament_id: 1, name: 'Team One', club: 'Club A', extra_fields: {} },
  { id: 2, tournament_id: 1, name: 'Team Two', club: 'Club B', extra_fields: {} },
]

function renderWatchPage() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/watch']}>
        <Routes>
          <Route path="/tournaments/:id/watch" element={<WatchPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('WatchPage', () => {
  it('renders ranked teams by name, in rank order, with time or status', async () => {
    vi.mocked(client.api.get).mockImplementation((path: string) => {
      if (path.includes('/standings')) return Promise.resolve(standings)
      if (path.includes('/teams')) return Promise.resolve(teams)
      return Promise.reject(new Error(`unexpected path ${path}`))
    })

    renderWatchPage()

    expect(await screen.findByText('Team One')).toBeInTheDocument()
    expect(screen.getByText('Team Two')).toBeInTheDocument()
    expect(screen.getByText('DNF')).toBeInTheDocument()

    const rows = screen.getAllByRole('row')
    // rows[0] is the header row; data rows follow in rank order.
    expect(rows[1]).toHaveTextContent('Team One')
    expect(rows[2]).toHaveTextContent('Team Two')
  })
})

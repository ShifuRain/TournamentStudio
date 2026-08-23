import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TeamsTab } from './TeamsTab'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})

function renderTab() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/teams']}>
        <Routes>
          <Route path="/tournaments/:id/teams" element={<TeamsTab />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TeamsTab', () => {
  it('renders teams and roster-field inputs, then submits a new team', async () => {
    vi.mocked(client.api.get).mockImplementation((path: unknown) => {
      const p = path as string
      if (p === '/api/tournaments/1') {
        return Promise.resolve({
          id: 1,
          name: 'T',
          sport_plugin_id: 'dragonboat',
          tournament_type_id: 'timed-heats-reseeding',
          language: 'en',
          status: 'draft',
        })
      }
      if (p === '/api/tournaments/1/teams') {
        return Promise.resolve([{ id: 5, tournament_id: 1, name: 'Rhein Dragons', club: '', extra_fields: {} }])
      }
      if (p === '/api/plugins') {
        return Promise.resolve({
          sports: [
            {
              id: 'dragonboat',
              display_name: 'Dragonboat',
              compatible_tournament_types: [],
              roster_fields: [{ key: 'boat_class', label: 'Boat class', required: false }],
            },
          ],
          tournament_types: [],
        })
      }
      return Promise.reject(new Error(`unexpected path ${p}`))
    })
    vi.mocked(client.api.post).mockResolvedValue({
      id: 6,
      tournament_id: 1,
      name: 'New Team',
      club: '',
      extra_fields: { boat_class: 'K1' },
    })

    renderTab()

    expect(await screen.findByText('Rhein Dragons')).toBeInTheDocument()
    expect(await screen.findByLabelText('Boat class')).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('teams_name'), 'New Team')
    await userEvent.type(screen.getByLabelText('Boat class'), 'K1')
    await userEvent.click(screen.getByText('teams_add_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/teams', {
        name: 'New Team',
        club: '',
        extra_fields: { boat_class: 'K1' },
      }),
    )
  })
})

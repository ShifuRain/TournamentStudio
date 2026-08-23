import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TournamentDetailPage } from './TournamentDetailPage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/teams']}>
        <Routes>
          <Route path="/tournaments/:id" element={<TournamentDetailPage />}>
            <Route path="teams" element={<div>teams content</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TournamentDetailPage', () => {
  it('renders the tournament name and the active teams tab content', async () => {
    vi.mocked(client.api.get).mockResolvedValue({
      id: 1,
      name: 'Herbstregatta',
      sport_plugin_id: 'dragonboat',
      tournament_type_id: 'timed-heats-reseeding',
      language: 'de',
      status: 'draft',
    })
    renderPage()

    expect(await screen.findByText('Herbstregatta')).toBeInTheDocument()
    expect(screen.getByText('teams content')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'tab_schedule' })).toBeInTheDocument()
  })
})

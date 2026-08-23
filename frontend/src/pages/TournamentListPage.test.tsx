import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TournamentListPage } from './TournamentListPage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ role: 'organizer' }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <TournamentListPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TournamentListPage', () => {
  it('renders tournaments from the API', async () => {
    vi.mocked(client.api.get).mockResolvedValue([
      {
        id: 1,
        name: 'Herbstregatta',
        sport_plugin_id: 'dragonboat',
        tournament_type_id: 'timed-heats-reseeding',
        language: 'de',
        status: 'draft',
      },
    ])
    renderPage()

    expect(await screen.findByText('Herbstregatta')).toBeInTheDocument()
  })

  it('shows a create-tournament link for organizers', async () => {
    vi.mocked(client.api.get).mockResolvedValue([])
    renderPage()

    expect(await screen.findByText('tournament_create_link')).toBeInTheDocument()
  })
})

import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TournamentCreatePage } from './TournamentCreatePage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../i18n/i18n', () => ({
  useAvailableLanguages: () => ['en', 'de'],
}))

const navigateMock = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => navigateMock }
})

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <TournamentCreatePage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TournamentCreatePage', () => {
  it('filters tournament types to the selected sport, and submits', async () => {
    vi.mocked(client.api.get).mockResolvedValue({
      sports: [
        {
          id: 'dragonboat',
          display_name: 'Dragonboat',
          compatible_tournament_types: ['timed-heats-reseeding'],
          roster_fields: [],
        },
      ],
      tournament_types: [
        { id: 'timed-heats-reseeding', compatible_sports: ['dragonboat'] },
        { id: 'knockout', compatible_sports: ['some-other-sport'] },
      ],
    })
    vi.mocked(client.api.post).mockResolvedValue({
      id: 42,
      name: 'Test',
      sport_plugin_id: 'dragonboat',
      tournament_type_id: 'timed-heats-reseeding',
      language: 'en',
      status: 'draft',
    })

    renderPage()

    await userEvent.type(await screen.findByLabelText('tournament_name'), 'Test')
    await userEvent.selectOptions(screen.getByLabelText('tournament_sport'), 'dragonboat')

    const typeSelect = screen.getByLabelText('tournament_type') as HTMLSelectElement
    const optionValues = Array.from(typeSelect.options).map((o) => o.value)
    expect(optionValues).toContain('timed-heats-reseeding')
    expect(optionValues).not.toContain('knockout')

    await userEvent.selectOptions(typeSelect, 'timed-heats-reseeding')
    await userEvent.click(screen.getByText('tournament_create_submit'))

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith('/tournaments/42'))
  })
})

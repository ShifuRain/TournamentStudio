import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleRoundActions } from './ScheduleRoundActions'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Round } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string, vars?: Record<string, unknown>) => (vars ? `${key} ${JSON.stringify(vars)}` : key) }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, post: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

const closedRound: Round = { id: 2, round_number: 2, status: 'closed', groups: [], divisions: [] }
const earlierRound: Round = { id: 1, round_number: 1, status: 'closed', groups: [], divisions: [] }

function renderActions(currentRound: Round, allRounds: Round[]) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route
            path="/tournaments/:id/schedule"
            element={<ScheduleRoundActions currentRound={currentRound} allRounds={allRounds} />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleRoundActions', () => {
  it('creates the next round', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.post).mockResolvedValue({ id: 3, round_number: 3, status: 'open', groups: [], divisions: [] })

    renderActions(closedRound, [earlierRound, closedRound])

    await userEvent.click(screen.getByText('schedule_round_actions_next'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/rounds/2/next'),
    )
  })

  it('cuts divisions with the entered cuts', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.post).mockResolvedValue({ divisions: [] })

    renderActions(closedRound, [earlierRound, closedRound])

    await userEvent.type(screen.getByLabelText('schedule_round_actions_divisions_name'), 'Gold')
    await userEvent.click(screen.getByText('schedule_round_actions_divisions_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/rounds/2/divisions', {
        cuts: [{ name: 'Gold', size: 1 }],
      }),
    )
  })

  it('does not render round actions for an open round', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderActions({ ...closedRound, status: 'open' }, [{ ...closedRound, status: 'open' }])

    expect(screen.queryByText('schedule_round_actions_next')).not.toBeInTheDocument()
  })

  it('renders round history for earlier rounds', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderActions(closedRound, [earlierRound, closedRound])

    expect(screen.getByText('schedule_round_history_title')).toBeInTheDocument()
  })

  it('does not render the divisions form once the round already has divisions', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    const roundWithDivisions: Round = {
      ...closedRound,
      divisions: [{ id: 1, name: 'Gold', team_ids: ['1', '2'] }],
    }
    renderActions(roundWithDivisions, [earlierRound, roundWithDivisions])

    expect(screen.queryByText('schedule_round_actions_divisions_title')).not.toBeInTheDocument()
    expect(screen.queryByText('schedule_round_actions_divisions_submit')).not.toBeInTheDocument()
  })

  it('disables the divisions submit button until a cut has a non-blank name', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })

    renderActions(closedRound, [earlierRound, closedRound])

    const submitButton = screen.getByText('schedule_round_actions_divisions_submit')
    expect(submitButton).toBeDisabled()

    await userEvent.type(screen.getByLabelText('schedule_round_actions_divisions_name'), 'Gold')

    expect(submitButton).toBeEnabled()
  })
})

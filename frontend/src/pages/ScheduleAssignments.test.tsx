import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleAssignments } from './ScheduleAssignments'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

function renderPanel(mode: 'group' | 'division') {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route
            path="/tournaments/:id/schedule"
            element={
              <ScheduleAssignments
                mode={mode}
                roundId={5}
                items={[
                  { id: 10, label: 'Group 1' },
                  { id: 11, label: 'Group 2' },
                ]}
              />
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleAssignments', () => {
  it('schedules group heats with the chosen courses', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [
        { id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 },
        { id: 2, tournament_id: 1, name: 'Course B', heat_interval_seconds: 300, delay_offset_seconds: 0 },
      ],
    })
    vi.mocked(client.api.post).mockResolvedValue({ heats: [] })

    renderPanel('group')

    await screen.findAllByText('Course A')
    await userEvent.selectOptions(screen.getByLabelText(/schedule_assignments_course_label — Group 1/), '1')
    await userEvent.selectOptions(screen.getByLabelText(/schedule_assignments_course_label — Group 2/), '2')
    await userEvent.click(screen.getByText('schedule_assignments_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/rounds/5/schedule', {
        assignments: [
          { group_id: 10, course_id: 1 },
          { group_id: 11, course_id: 2 },
        ],
      }),
    )
  })

  it('schedules division heats against the divisions/schedule endpoint', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [{ id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 }],
    })
    vi.mocked(client.api.post).mockResolvedValue({ heats: [] })

    renderPanel('division')

    await screen.findAllByText('Course A')
    await userEvent.selectOptions(screen.getByLabelText(/schedule_assignments_course_label — Group 1/), '1')
    await userEvent.selectOptions(screen.getByLabelText(/schedule_assignments_course_label — Group 2/), '1')
    await userEvent.click(screen.getByText('schedule_assignments_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith(
        '/api/tournaments/1/divisions/schedule',
        expect.objectContaining({
          assignments: [
            { division_id: 10, course_id: 1 },
            { division_id: 11, course_id: 1 },
          ],
        }),
      ),
    )
  })

  it('renders nothing when there are no items to schedule', () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({ courses: [] })

    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
          <Routes>
            <Route
              path="/tournaments/:id/schedule"
              element={<ScheduleAssignments mode="group" roundId={5} items={[]} />}
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(container).toBeEmptyDOMElement()
  })
})

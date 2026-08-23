import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ScheduleCourses } from './ScheduleCourses'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), post: vi.fn(), patch: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

function renderCourses() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/schedule']}>
        <Routes>
          <Route path="/tournaments/:id/schedule" element={<ScheduleCourses />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ScheduleCourses', () => {
  it('renders courses and submits a new one', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [{ id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 }],
    })
    vi.mocked(client.api.post).mockResolvedValue({
      id: 2,
      tournament_id: 1,
      name: 'Course B',
      heat_interval_seconds: 240,
      delay_offset_seconds: 0,
    })

    renderCourses()

    expect(await screen.findByText(/Course A/)).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('schedule_courses_name'), 'Course B')
    await userEvent.clear(screen.getByLabelText('schedule_courses_interval'))
    await userEvent.type(screen.getByLabelText('schedule_courses_interval'), '240')
    await userEvent.click(screen.getByText('schedule_courses_add_submit'))

    await waitFor(() =>
      expect(client.api.post).toHaveBeenCalledWith('/api/tournaments/1/courses', {
        name: 'Course B',
        heat_interval_seconds: 240,
      }),
    )
  })

  it('shows an error message when adding a course fails', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({ courses: [] })
    vi.mocked(client.api.post).mockRejectedValue(new Error('server error'))

    renderCourses()

    await userEvent.type(await screen.findByLabelText('schedule_courses_name'), 'Course B')
    await userEvent.click(screen.getByText('schedule_courses_add_submit'))

    expect(await screen.findByRole('alert')).toHaveTextContent('schedule_courses_add_error')
  })

  it('hides the add-course form for non-organizer roles', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'time_entry', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue({
      courses: [{ id: 1, tournament_id: 1, name: 'Course A', heat_interval_seconds: 300, delay_offset_seconds: 0 }],
    })

    renderCourses()

    expect(await screen.findByText(/Course A/)).toBeInTheDocument()
    expect(screen.queryByLabelText('schedule_courses_name')).not.toBeInTheDocument()
  })
})

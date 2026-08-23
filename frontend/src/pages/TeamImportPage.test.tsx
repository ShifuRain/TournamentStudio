import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TeamImportPage } from './TeamImportPage'
import * as client from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, postForm: vi.fn() } }
})

function renderPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/tournaments/1/teams/import']}>
        <Routes>
          <Route path="/tournaments/:id/teams/import" element={<TeamImportPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TeamImportPage', () => {
  it('uploads a file and shows the import results', async () => {
    vi.mocked(client.api.postForm).mockResolvedValue({
      imported: 2,
      problems: [{ row_index: 3, message: 'missing team name' }],
    })

    renderPage()

    const file = new File(['name,club\nA,B'], 'teams.csv', { type: 'text/csv' })
    const input = screen.getByLabelText('import_file_label')
    await userEvent.upload(input, file)
    await userEvent.click(screen.getByText('import_submit'))

    await waitFor(() => expect(client.api.postForm).toHaveBeenCalled())
    expect(await screen.findByText(/import_result_summary/)).toBeInTheDocument()
    expect(screen.getByText(/import_row_problem/)).toBeInTheDocument()

    const [path, form] = vi.mocked(client.api.postForm).mock.calls[0]
    expect(path).toBe('/api/tournaments/1/teams/import')
    expect(form.get('file')).toBeInstanceOf(File)
  })
})

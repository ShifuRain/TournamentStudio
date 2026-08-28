import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PluginsPage } from './PluginsPage'
import * as client from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { PluginsResponse } from '../api/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api, get: vi.fn(), postForm: vi.fn(), delete: vi.fn() } }
})
vi.mock('../auth/AuthContext', () => ({ useAuth: vi.fn() }))

const catalog: PluginsResponse = {
  sports: [
    { id: 'dragonboat', display_name: 'Dragonboat', compatible_tournament_types: [], roster_fields: [], source: 'bundled' },
    { id: 'extra-sport', display_name: 'Extra Sport', compatible_tournament_types: [], roster_fields: [], source: 'extra-sport.lua' },
  ],
  tournament_types: [
    { id: 'timed-heats-reseeding', compatible_sports: [], source: 'bundled' },
  ],
}

function renderPluginsPage() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/plugins']}>
        <Routes>
          <Route path="/plugins" element={<PluginsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('PluginsPage', () => {
  it('shows a Built-in badge for bundled plugins and no delete button', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)

    renderPluginsPage()

    expect(await screen.findByText('Dragonboat')).toBeInTheDocument()
    const bundledRow = screen.getByText('Dragonboat').closest('li')
    expect(bundledRow).toHaveTextContent('plugins_builtin_badge')
    expect(bundledRow?.querySelector('button')).toBeNull()
  })

  it('shows a delete button for an external plugin when organizer, and calls DELETE with its filename', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)
    vi.mocked(client.api.delete).mockResolvedValue(undefined)

    renderPluginsPage()

    const externalRow = (await screen.findByText('Extra Sport')).closest('li')
    const deleteButton = externalRow?.querySelector('button')
    expect(deleteButton).not.toBeNull()
    await userEvent.click(deleteButton as HTMLButtonElement)

    await waitFor(() => expect(client.api.delete).toHaveBeenCalledWith('/api/plugins/extra-sport.lua'))
  })

  it('hides the upload form and delete buttons for a non-organizer', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'spectator', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)

    renderPluginsPage()

    await screen.findByText('Dragonboat')
    expect(screen.queryByText('plugins_upload_submit')).toBeNull()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('shows the upload error message returned by the backend', async () => {
    vi.mocked(useAuth).mockReturnValue({ role: 'organizer', token: 'x', login: vi.fn(), logout: vi.fn() })
    vi.mocked(client.api.get).mockResolvedValue(catalog)
    vi.mocked(client.api.postForm).mockRejectedValue(new Error('invalid plugin: load broken.lua: parse error'))

    renderPluginsPage()

    await screen.findByText('Dragonboat')
    const file = new File(['not lua'], 'broken.lua', { type: 'text/plain' })
    const input = screen.getByLabelText('plugins_upload_file_label')
    await userEvent.upload(input, file)
    await userEvent.click(screen.getByText('plugins_upload_submit'))

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid plugin: load broken.lua: parse error')
  })
})

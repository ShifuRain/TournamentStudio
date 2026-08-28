import { useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { PluginsResponse } from '../api/types'

export function PluginsPage() {
  const { t } = useTranslation()
  const { role } = useAuth()
  const queryClient = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const isOrganizer = role === 'organizer'

  const { data } = useQuery({
    queryKey: ['plugins'],
    queryFn: () => api.get<PluginsResponse>('/api/plugins'),
  })
  const sports = data?.sports ?? []
  const tournamentTypes = data?.tournament_types ?? []

  const uploadMutation = useMutation({
    mutationFn: () => {
      const form = new FormData()
      form.append('file', file as File)
      return api.postForm<{ filename: string }>('/api/plugins', form)
    },
    onSuccess: () => {
      setFile(null)
      void queryClient.invalidateQueries({ queryKey: ['plugins'] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (filename: string) => api.delete(`/api/plugins/${encodeURIComponent(filename)}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['plugins'] })
    },
  })

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    setFile(e.target.files?.[0] ?? null)
  }

  return (
    <div className="mx-auto max-w-3xl p-8">
      <h1 className="mb-6 text-xl font-bold">{t('plugins_title')}</h1>

      <section className="mb-8">
        <h2 className="mb-2 text-lg font-semibold">{t('plugins_sports_title')}</h2>
        <ul className="space-y-2">
          {sports.map((sp) => (
            <li key={sp.id} className="flex items-center justify-between rounded border p-3">
              <span className="font-medium">{sp.display_name}</span>
              {sp.source === 'bundled' ? (
                <span className="text-xs text-gray-400">{t('plugins_builtin_badge')}</span>
              ) : (
                isOrganizer && (
                  <button onClick={() => deleteMutation.mutate(sp.source)} className="text-xs text-red-600">
                    {t('plugins_delete')}
                  </button>
                )
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="mb-8">
        <h2 className="mb-2 text-lg font-semibold">{t('plugins_tournament_types_title')}</h2>
        <ul className="space-y-2">
          {tournamentTypes.map((ttp) => (
            <li key={ttp.id} className="flex items-center justify-between rounded border p-3">
              <span className="font-medium">{ttp.id}</span>
              {ttp.source === 'bundled' ? (
                <span className="text-xs text-gray-400">{t('plugins_builtin_badge')}</span>
              ) : (
                isOrganizer && (
                  <button onClick={() => deleteMutation.mutate(ttp.source)} className="text-xs text-red-600">
                    {t('plugins_delete')}
                  </button>
                )
              )}
            </li>
          ))}
        </ul>
      </section>

      {isOrganizer && (
        <section>
          <h2 className="mb-2 text-lg font-semibold">{t('plugins_upload_title')}</h2>
          <div className="space-y-2">
            <input type="file" accept=".lua" aria-label={t('plugins_upload_file_label')} onChange={handleFileChange} />
            <div>
              <button
                onClick={() => file && uploadMutation.mutate()}
                disabled={!file || uploadMutation.isPending}
                className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
              >
                {t('plugins_upload_submit')}
              </button>
            </div>
            {uploadMutation.isError && (
              <p role="alert" className="text-sm text-red-600">
                {(uploadMutation.error as Error).message}
              </p>
            )}
          </div>
        </section>
      )}
    </div>
  )
}

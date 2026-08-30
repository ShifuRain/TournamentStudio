import { useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { PluginsResponse } from '../api/types'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'
import { StatusChip } from '../components/ui/StatusChip'

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
    <div className={ui.page}>
      <h1 className={`mb-6 ${ui.h1}`}>{t('plugins_title')}</h1>

      <section className="mb-8">
        <h2 className={`mb-2 ${ui.h2}`}>{t('plugins_sports_title')}</h2>
        <ul className="space-y-2">
          {sports.map((sp) => (
            <li key={sp.id} className={`flex items-center justify-between ${ui.panel}`}>
              <span className="font-medium">{sp.display_name}</span>
              {sp.source === 'bundled' ? (
                <StatusChip>{t('plugins_builtin_badge')}</StatusChip>
              ) : (
                isOrganizer && (
                  <Button variant="ghost" size="sm" onClick={() => deleteMutation.mutate(sp.source)}>
                    {t('plugins_delete')}
                  </Button>
                )
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="mb-8">
        <h2 className={`mb-2 ${ui.h2}`}>{t('plugins_tournament_types_title')}</h2>
        <ul className="space-y-2">
          {tournamentTypes.map((ttp) => (
            <li key={ttp.id} className={`flex items-center justify-between ${ui.panel}`}>
              <span className="font-medium">{ttp.id}</span>
              {ttp.source === 'bundled' ? (
                <StatusChip>{t('plugins_builtin_badge')}</StatusChip>
              ) : (
                isOrganizer && (
                  <Button variant="ghost" size="sm" onClick={() => deleteMutation.mutate(ttp.source)}>
                    {t('plugins_delete')}
                  </Button>
                )
              )}
            </li>
          ))}
        </ul>
      </section>

      {isOrganizer && (
        <section className={ui.panel}>
          <h2 className={`mb-2 ${ui.h2}`}>{t('plugins_upload_title')}</h2>
          <div className="space-y-2">
            <input
              type="file"
              accept=".lua"
              aria-label={t('plugins_upload_file_label')}
              onChange={handleFileChange}
              className="font-mono text-sm text-foam-dim file:mr-3 file:rounded file:border file:border-hairline file:bg-navy-2/40 file:px-3 file:py-1.5 file:text-foam"
            />
            <div>
              <Button onClick={() => file && uploadMutation.mutate()} disabled={!file || uploadMutation.isPending} size="sm">
                {t('plugins_upload_submit')}
              </Button>
            </div>
            {uploadMutation.isError && (
              <p role="alert" className={ui.error}>
                {(uploadMutation.error as Error).message}
              </p>
            )}
          </div>
        </section>
      )}
    </div>
  )
}

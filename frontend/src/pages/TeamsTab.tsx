import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { PluginsResponse, Team, Tournament } from '../api/types'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'

export function TeamsTab() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()

  const { data: tournament } = useQuery({
    queryKey: ['tournament', id],
    queryFn: () => api.get<Tournament>(`/api/tournaments/${id}`),
    enabled: !!id,
  })
  const { data: teams, isLoading } = useQuery({
    queryKey: ['teams', id],
    queryFn: () => api.get<Team[]>(`/api/tournaments/${id}/teams`),
    enabled: !!id,
  })
  const { data: plugins } = useQuery({
    queryKey: ['plugins'],
    queryFn: () => api.get<PluginsResponse>('/api/plugins'),
  })

  const rosterFields = plugins?.sports.find((s) => s.id === tournament?.sport_plugin_id)?.roster_fields ?? []

  const [name, setName] = useState('')
  const [club, setClub] = useState('')
  const [extraFields, setExtraFields] = useState<Record<string, string>>({})

  const createMutation = useMutation({
    mutationFn: () => api.post<Team>(`/api/tournaments/${id}/teams`, { name, club, extra_fields: extraFields }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['teams', id] })
      setName('')
      setClub('')
      setExtraFields({})
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    createMutation.mutate()
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h2 className={ui.h2}>{t('teams_title')}</h2>
        <Link to="import" className={ui.link}>
          {t('teams_import_link')}
        </Link>
      </div>

      {isLoading && <p className={ui.muted}>{t('loading')}</p>}
      <ul className={`mb-6 ${ui.panel} ${ui.divider} p-0`}>
        {teams?.map((team) => (
          <li key={team.id} className="px-4 py-3">
            <span className="font-medium">{team.name}</span>
            {team.club && <span className={`ml-2 text-sm ${ui.faint}`}>{team.club}</span>}
          </li>
        ))}
      </ul>

      <form onSubmit={handleSubmit} className={`max-w-md space-y-3 ${ui.panel}`}>
        <h3 className={ui.h3}>{t('teams_add_title')}</h3>
        <div>
          <label htmlFor="team-name" className={ui.label}>
            {t('teams_name')}
          </label>
          <input
            id="team-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={ui.input}
            required
          />
        </div>
        <div>
          <label htmlFor="team-club" className={ui.label}>
            {t('teams_club')}
          </label>
          <input id="team-club" value={club} onChange={(e) => setClub(e.target.value)} className={ui.input} />
        </div>
        {rosterFields.map((field) => (
          <div key={field.key}>
            <label htmlFor={`field-${field.key}`} className={ui.label}>
              {field.label}
            </label>
            <input
              id={`field-${field.key}`}
              value={extraFields[field.key] ?? ''}
              onChange={(e) => setExtraFields((prev) => ({ ...prev, [field.key]: e.target.value }))}
              className={ui.input}
              required={field.required}
            />
          </div>
        ))}
        {createMutation.isError && (
          <p role="alert" className={ui.error}>
            {t('teams_add_error')}
          </p>
        )}
        <Button type="submit" disabled={createMutation.isPending} size="sm">
          {t('teams_add_submit')}
        </Button>
      </form>
    </div>
  )
}

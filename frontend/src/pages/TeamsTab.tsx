import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { PluginsResponse, Team, Tournament } from '../api/types'

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
        <h2 className="text-lg font-semibold">{t('teams_title')}</h2>
        <Link to="import" className="text-sm text-blue-600">
          {t('teams_import_link')}
        </Link>
      </div>

      {isLoading && <p>{t('loading')}</p>}
      <ul className="mb-6 divide-y rounded border bg-white">
        {teams?.map((team) => (
          <li key={team.id} className="px-4 py-3">
            <span className="font-medium">{team.name}</span>
            {team.club && <span className="ml-2 text-sm text-gray-500">{team.club}</span>}
          </li>
        ))}
      </ul>

      <form onSubmit={handleSubmit} className="max-w-md space-y-3 rounded border bg-white p-4">
        <h3 className="font-medium">{t('teams_add_title')}</h3>
        <div>
          <label htmlFor="team-name" className="block text-sm font-medium">
            {t('teams_name')}
          </label>
          <input
            id="team-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
            required
          />
        </div>
        <div>
          <label htmlFor="team-club" className="block text-sm font-medium">
            {t('teams_club')}
          </label>
          <input
            id="team-club"
            value={club}
            onChange={(e) => setClub(e.target.value)}
            className="mt-1 w-full rounded border px-3 py-2"
          />
        </div>
        {rosterFields.map((field) => (
          <div key={field.key}>
            <label htmlFor={`field-${field.key}`} className="block text-sm font-medium">
              {field.label}
            </label>
            <input
              id={`field-${field.key}`}
              value={extraFields[field.key] ?? ''}
              onChange={(e) => setExtraFields((prev) => ({ ...prev, [field.key]: e.target.value }))}
              className="mt-1 w-full rounded border px-3 py-2"
              required={field.required}
            />
          </div>
        ))}
        {createMutation.isError && (
          <p role="alert" className="text-sm text-red-600">
            {t('teams_add_error')}
          </p>
        )}
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
        >
          {t('teams_add_submit')}
        </button>
      </form>
    </div>
  )
}

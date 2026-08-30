import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { PluginsResponse, Tournament } from '../api/types'
import { useAvailableLanguages } from '../i18n/i18n'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'

export function TournamentCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const availableLanguages = useAvailableLanguages()

  const { data: plugins } = useQuery({
    queryKey: ['plugins'],
    queryFn: () => api.get<PluginsResponse>('/api/plugins'),
  })

  const [name, setName] = useState('')
  const [language, setLanguage] = useState('en')
  const [sportId, setSportId] = useState('')
  const [typeId, setTypeId] = useState('')

  const selectedSport = plugins?.sports.find((s) => s.id === sportId)
  const compatibleTypes =
    plugins?.tournament_types.filter((tt) => selectedSport?.compatible_tournament_types.includes(tt.id)) ?? []

  const createMutation = useMutation({
    mutationFn: () =>
      api.post<Tournament>('/api/tournaments', {
        name,
        sport_plugin_id: sportId,
        tournament_type_id: typeId,
        language,
      }),
    onSuccess: (tournament) => {
      void queryClient.invalidateQueries({ queryKey: ['tournaments'] })
      navigate(`/tournaments/${tournament.id}`)
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    createMutation.mutate()
  }

  return (
    <div className={ui.pageNarrow}>
      <h1 className={`mb-6 ${ui.h1}`}>{t('tournament_create_title')}</h1>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label htmlFor="name" className={ui.label}>
            {t('tournament_name')}
          </label>
          <input id="name" value={name} onChange={(e) => setName(e.target.value)} className={ui.input} required />
        </div>
        <div>
          <label htmlFor="language" className={ui.label}>
            {t('tournament_language')}
          </label>
          <select
            id="language"
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
            className={`w-full ${ui.select}`}
          >
            {availableLanguages.map((lang) => (
              <option key={lang} value={lang}>
                {lang.toUpperCase()}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="sport" className={ui.label}>
            {t('tournament_sport')}
          </label>
          <select
            id="sport"
            value={sportId}
            onChange={(e) => {
              setSportId(e.target.value)
              setTypeId('')
            }}
            className={`w-full ${ui.select}`}
            required
          >
            <option value="" disabled>
              {t('tournament_select_sport')}
            </option>
            {plugins?.sports.map((sport) => (
              <option key={sport.id} value={sport.id}>
                {sport.display_name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="type" className={ui.label}>
            {t('tournament_type')}
          </label>
          <select
            id="type"
            value={typeId}
            onChange={(e) => setTypeId(e.target.value)}
            className={`w-full ${ui.select}`}
            required
            disabled={!sportId}
          >
            <option value="" disabled>
              {t('tournament_select_type')}
            </option>
            {compatibleTypes.map((tt) => (
              <option key={tt.id} value={tt.id}>
                {tt.id}
              </option>
            ))}
          </select>
        </div>
        {createMutation.isError && (
          <p role="alert" className={ui.error}>
            {t('tournament_create_error')}
          </p>
        )}
        <Button type="submit" disabled={createMutation.isPending} className="w-full">
          {t('tournament_create_submit')}
        </Button>
      </form>
    </div>
  )
}

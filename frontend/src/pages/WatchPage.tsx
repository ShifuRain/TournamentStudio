import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { StandingsResponse, Team } from '../api/types'
import { useTournamentSocket } from '../hooks/useTournamentSocket'

function teamName(teamId: string, teams: Team[]): string {
  return teams.find((team) => String(team.id) === teamId)?.name ?? teamId
}

export function WatchPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { connectionLost } = useTournamentSocket(id)

  const { data: standingsData } = useQuery({
    queryKey: ['standings', id],
    queryFn: () => api.get<StandingsResponse>(`/api/tournaments/${id}/standings`),
    enabled: !!id,
  })
  const rounds = standingsData?.rounds ?? []

  const { data: teamsData } = useQuery({
    queryKey: ['teams', id],
    queryFn: () => api.get<Team[]>(`/api/tournaments/${id}/teams`),
    enabled: !!id,
  })
  const teams = teamsData ?? []

  return (
    <div className="mx-auto max-w-3xl p-8">
      {connectionLost && (
        <p role="status" className="mb-4 rounded bg-yellow-50 p-2 text-sm text-yellow-800">
          {t('watch_connection_lost')}
        </p>
      )}
      {[...rounds].reverse().map((round) => (
        <section key={round.id} className="mb-8">
          <h2 className="mb-4 text-lg font-semibold">
            {t('schedule_round_history_entry', { number: round.round_number, status: round.status })}
          </h2>
          {round.standings.map((entry, index) => (
            <table key={entry.group_id ?? entry.division_id} className="mb-4 w-full text-sm">
              <caption className="mb-2 text-left font-medium">
                {entry.division_name ?? t('schedule_round_create_group_label', { number: index + 1 })}
              </caption>
              <thead>
                <tr className="border-b text-left text-gray-500">
                  <th className="py-1">{t('watch_rank')}</th>
                  <th className="py-1">{t('watch_team')}</th>
                  <th className="py-1">{t('watch_time_or_status')}</th>
                </tr>
              </thead>
              <tbody>
                {entry.ranked_teams.map((rt) => (
                  <tr key={rt.team_id}>
                    <td className="py-1">{rt.rank}</td>
                    <td className="py-1">{teamName(rt.team_id, teams)}</td>
                    <td className="py-1">{rt.status || `${rt.time_seconds}s`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ))}
        </section>
      ))}
    </div>
  )
}

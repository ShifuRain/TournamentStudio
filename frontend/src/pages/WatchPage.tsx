import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { RankedTeam, StandingsResponse, Team } from '../api/types'
import { useTournamentSocket } from '../hooks/useTournamentSocket'
import { useFlipOnChange } from '../hooks/useFlipOnChange'
import { ui } from '../components/ui/styles'
import { StatusChip } from '../components/ui/StatusChip'

function teamName(teamId: string, teams: Team[]): string {
  return teams.find((team) => String(team.id) === teamId)?.name ?? teamId
}

function RankedTeamRow({ rt, teams }: { rt: RankedTeam; teams: Team[] }) {
  const resultText = rt.status || `${rt.time_seconds}s`
  const flipping = useFlipOnChange(resultText)

  return (
    <tr>
      <td className="py-1 font-mono text-teal-tint">{rt.rank}</td>
      <td className="py-1">{teamName(rt.team_id, teams)}</td>
      <td className="py-1">
        {rt.status ? (
          <StatusChip tone="warn">{rt.status}</StatusChip>
        ) : (
          <span className={`flip-value font-mono tabular-nums ${flipping ? 'flipping' : ''}`}>{resultText}</span>
        )}
      </td>
    </tr>
  )
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
    <div className={ui.page}>
      <div className="mb-6 flex items-center justify-between">
        <span className={ui.eyebrow}>{t('tab_standings')}</span>
        {!connectionLost && (
          <span className="inline-flex items-center gap-1.5 rounded border border-red-tint/40 bg-red/10 px-2 py-0.5 font-mono text-xs tracking-widest text-red-tint">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-red-tint" />
            LIVE
          </span>
        )}
      </div>
      {connectionLost && (
        <p role="status" className={`mb-4 rounded border border-yellow/40 bg-yellow/10 p-2 text-sm text-yellow`}>
          {t('watch_connection_lost')}
        </p>
      )}
      {[...rounds].reverse().map((round) => (
        <section key={round.id} className="mb-8">
          <h2 className={`mb-4 ${ui.h2}`}>
            {t('schedule_round_history_entry', { number: round.round_number, status: round.status })}
          </h2>
          {round.standings.map((entry, index) => (
            <table key={entry.group_id ?? entry.division_id} className={`mb-4 w-full text-sm ${ui.panel} border-collapse`}>
              <caption className="mb-2 text-left font-display text-base font-bold uppercase tracking-wide text-foam">
                {entry.division_name ?? t('schedule_round_create_group_label', { number: index + 1 })}
              </caption>
              <thead>
                <tr className={ui.tableHead}>
                  <th className="py-1">{t('watch_rank')}</th>
                  <th className="py-1">{t('watch_team')}</th>
                  <th className="py-1">{t('watch_time_or_status')}</th>
                </tr>
              </thead>
              <tbody className={ui.divider}>
                {entry.ranked_teams.map((rt) => (
                  <RankedTeamRow key={rt.team_id} rt={rt} teams={teams} />
                ))}
              </tbody>
            </table>
          ))}
        </section>
      ))}
    </div>
  )
}

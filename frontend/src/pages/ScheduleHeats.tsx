import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course, Heat, Round, Team } from '../api/types'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'
import { StatusChip } from '../components/ui/StatusChip'

interface ScheduleHeatsProps {
  heats: Heat[]
  courses: Course[]
  currentRound: Round
  teams: Team[]
}

type ResultEntry = { timeSeconds: string; status: string }

function teamIdsForHeat(heat: Heat, round: Round): string[] {
  if (heat.group_id !== null) {
    return round.groups.find((g) => g.id === heat.group_id)?.team_ids ?? []
  }
  return round.divisions.find((d) => d.id === heat.division_id)?.team_ids ?? []
}

function teamName(teamId: string, teams: Team[]): string {
  return teams.find((team) => String(team.id) === teamId)?.name ?? teamId
}

function HeatRow({
  heat,
  courseName,
  round,
  teams,
}: {
  heat: Heat
  courseName: string
  round: Round
  teams: Team[]
}) {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()
  const teamIds = teamIdsForHeat(heat, round)

  const [start, setStart] = useState(heat.planned_start)
  const startMutation = useMutation({
    mutationFn: () => api.patch(`/api/tournaments/${id}/heats/${heat.id}`, { planned_start: start }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['schedule', id] })
    },
  })

  const [entries, setEntries] = useState<Record<string, ResultEntry>>({})
  const resultsMutation = useMutation({
    mutationFn: () => {
      const body: Record<string, { time_seconds?: number; status?: string }> = {}
      for (const teamId of teamIds) {
        const entry = entries[teamId]
        if (!entry) continue
        if (entry.status) {
          body[teamId] = { status: entry.status }
        } else if (entry.timeSeconds) {
          body[teamId] = { time_seconds: Number(entry.timeSeconds) }
        }
      }
      return api.post(`/api/tournaments/${id}/heats/${heat.id}/results`, body)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['schedule', id] })
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  const canEnterResults = role === 'organizer' || role === 'time_entry'
  const isOpen = heat.status !== 'closed'

  return (
    <li className="rounded-md border border-hairline p-3">
      <div className="mb-2 flex items-center justify-between font-mono text-xs text-foam-dim">
        <span>
          {courseName} — {new Date(heat.effective_start).toLocaleString()} —{' '}
          <StatusChip tone={heat.status === 'closed' ? 'good' : 'accent'}>
            {t(heat.status === 'closed' ? 'schedule_heats_status_closed' : 'schedule_heats_status_scheduled')}
          </StatusChip>
        </span>
        {role === 'organizer' && (
          <div className="flex items-center gap-2">
            <label htmlFor={`start-${heat.id}`}>{t('schedule_heats_start_label')}</label>
            <input
              id={`start-${heat.id}`}
              value={start}
              onChange={(e) => setStart(e.target.value)}
              className={`text-xs ${ui.input}`}
            />
            <Button type="button" variant="outline" size="sm" onClick={() => startMutation.mutate()}>
              {t('schedule_heats_start_override_submit')}
            </Button>
          </div>
        )}
      </div>
      {startMutation.isError && (
        <p role="alert" className={`mb-2 text-xs ${ui.error}`}>
          {t('schedule_heats_start_override_error')}
        </p>
      )}

      {isOpen && canEnterResults && (
        <>
          <table className="w-full text-sm">
            <tbody>
              {teamIds.map((teamId) => (
                <tr key={teamId}>
                  <td className="py-1">{teamName(teamId, teams)}</td>
                  <td>
                    <label htmlFor={`time-${heat.id}-${teamId}`} className="sr-only">
                      {t('schedule_heats_results_time')} — {teamId}
                    </label>
                    <input
                      id={`time-${heat.id}-${teamId}`}
                      placeholder={t('schedule_heats_results_time')}
                      value={entries[teamId]?.timeSeconds ?? ''}
                      onChange={(e) =>
                        setEntries((prev) => ({
                          ...prev,
                          [teamId]: { timeSeconds: e.target.value, status: '' },
                        }))
                      }
                      className={`w-28 text-center font-mono text-base tabular-nums ${ui.input}`}
                    />
                  </td>
                  <td>
                    <label htmlFor={`status-${heat.id}-${teamId}`} className="sr-only">
                      {t('schedule_heats_results_status')} — {teamId}
                    </label>
                    <select
                      id={`status-${heat.id}-${teamId}`}
                      value={entries[teamId]?.status ?? ''}
                      onChange={(e) =>
                        setEntries((prev) => ({
                          ...prev,
                          [teamId]: { timeSeconds: '', status: e.target.value },
                        }))
                      }
                      className={ui.select}
                    >
                      <option value="">{t('schedule_heats_results_status_finished')}</option>
                      <option value="DNF">DNF</option>
                      <option value="DSQ">DSQ</option>
                      <option value="DNS">DNS</option>
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {resultsMutation.isError && (
            <p role="alert" className={`mt-2 text-xs ${ui.error}`}>
              {t('schedule_heats_results_error')}
            </p>
          )}
          <Button type="button" onClick={() => resultsMutation.mutate()} disabled={resultsMutation.isPending} className="mt-2">
            {t('schedule_heats_results_submit')}
          </Button>
        </>
      )}
    </li>
  )
}

export function ScheduleHeats({ heats, courses, currentRound, teams }: ScheduleHeatsProps) {
  const { t } = useTranslation()

  if (heats.length === 0) {
    return null
  }

  function courseName(courseId: number): string {
    return courses.find((c) => c.id === courseId)?.name ?? String(courseId)
  }

  return (
    <section className={`mb-6 ${ui.panel}`}>
      <h2 className={`mb-3 ${ui.h2}`}>{t('schedule_heats_title')}</h2>
      <ul className="space-y-3">
        {heats.map((heat) => (
          <HeatRow
            key={heat.id}
            heat={heat}
            courseName={courseName(heat.course_id)}
            round={currentRound}
            teams={teams}
          />
        ))}
      </ul>
    </section>
  )
}

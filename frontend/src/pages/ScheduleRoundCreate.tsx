import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Round, Team } from '../api/types'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'

function shuffledGroups(teamIds: string[], groupCount: number): string[][] {
  const shuffled = [...teamIds]
  for (let i = shuffled.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]]
  }
  const groups: string[][] = Array.from({ length: groupCount }, () => [])
  shuffled.forEach((teamId, index) => {
    groups[index % groupCount].push(teamId)
  })
  return groups
}

export function ScheduleRoundCreate() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const { data: teamsData } = useQuery({
    queryKey: ['teams', id],
    queryFn: () => api.get<Team[]>(`/api/tournaments/${id}/teams`),
    enabled: !!id,
  })
  const teams = teamsData ?? []

  const [groupCount, setGroupCount] = useState(2)
  const [groups, setGroups] = useState<string[][] | null>(null)

  function teamName(teamId: string): string {
    return teams.find((team) => String(team.id) === teamId)?.name ?? teamId
  }

  function handleShuffle() {
    setGroups(shuffledGroups(teams.map((team) => String(team.id)), groupCount))
  }

  function moveTeam(teamId: string, toGroupIndex: number) {
    setGroups((prev) => {
      if (!prev) return prev
      const next = prev.map((group) => group.filter((id2) => id2 !== teamId))
      next[toGroupIndex].push(teamId)
      return next
    })
  }

  const createMutation = useMutation({
    mutationFn: () => api.post<Round>(`/api/tournaments/${id}/rounds`, { round_number: 1, groups }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  if (role !== 'organizer') {
    return null
  }

  return (
    <section className={`mb-6 ${ui.panel}`}>
      <h2 className={`mb-3 ${ui.h2}`}>{t('schedule_round_create_title')}</h2>
      <div className="mb-3 flex items-end gap-3">
        <div>
          <label htmlFor="group-count" className={ui.label}>
            {t('schedule_round_create_group_count')}
          </label>
          <input
            id="group-count"
            type="number"
            min={1}
            value={groupCount}
            onChange={(e) => setGroupCount(Number(e.target.value))}
            className={`w-24 ${ui.input}`}
          />
        </div>
        <Button type="button" variant="outline" onClick={handleShuffle} disabled={teams.length === 0}>
          {t('schedule_round_create_shuffle')}
        </Button>
      </div>

      {groups && (
        <>
          <div className="mb-4 grid grid-cols-2 gap-4">
            {groups.map((group, groupIndex) => (
              <div key={groupIndex} className="rounded-md border border-hairline p-3">
                <h3 className={`mb-2 ${ui.h3}`}>{t('schedule_round_create_group_label', { number: groupIndex + 1 })}</h3>
                <ul className="space-y-1">
                  {group.map((teamId) => (
                    <li key={teamId} className="flex items-center justify-between text-sm">
                      <span>{teamName(teamId)}</span>
                      <select
                        aria-label={`${t('schedule_round_create_move_to')} — ${teamName(teamId)}`}
                        value={groupIndex}
                        onChange={(e) => moveTeam(teamId, Number(e.target.value))}
                        className={`text-xs ${ui.select}`}
                      >
                        {groups.map((_, targetIndex) => (
                          <option key={targetIndex} value={targetIndex}>
                            {t('schedule_round_create_group_label', { number: targetIndex + 1 })}
                          </option>
                        ))}
                      </select>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
          {createMutation.isError && (
            <p role="alert" className={`mb-2 ${ui.error}`}>
              {t('schedule_round_create_error')}
            </p>
          )}
          <Button type="button" onClick={() => createMutation.mutate()} disabled={createMutation.isPending}>
            {t('schedule_round_create_submit')}
          </Button>
        </>
      )}
    </section>
  )
}

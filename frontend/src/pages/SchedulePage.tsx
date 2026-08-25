import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Course, Heat, Round, Team } from '../api/types'
import { ScheduleCourses } from './ScheduleCourses'
import { ScheduleRoundCreate } from './ScheduleRoundCreate'
import { ScheduleAssignments } from './ScheduleAssignments'
import { ScheduleHeats } from './ScheduleHeats'
import { ScheduleRoundActions } from './ScheduleRoundActions'

export function SchedulePage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()

  const { data: roundsData } = useQuery({
    queryKey: ['rounds', id],
    queryFn: () => api.get<{ rounds: Round[] }>(`/api/tournaments/${id}/rounds`),
    enabled: !!id,
  })
  const rounds = roundsData?.rounds ?? []
  const currentRound = rounds[rounds.length - 1]

  const { data: coursesData } = useQuery({
    queryKey: ['courses', id],
    queryFn: () => api.get<{ courses: Course[] }>(`/api/tournaments/${id}/courses`),
    enabled: !!id,
  })
  const courses = coursesData?.courses ?? []

  const { data: scheduleData } = useQuery({
    queryKey: ['schedule', id],
    queryFn: () => api.get<{ heats: Heat[] }>(`/api/tournaments/${id}/schedule`),
    enabled: !!id,
  })
  const heats = scheduleData?.heats ?? []
  const currentRoundHeats = currentRound ? heats.filter((h) => h.round_id === currentRound.id) : []

  const { data: teamsData } = useQuery({
    queryKey: ['teams', id],
    queryFn: () => api.get<Team[]>(`/api/tournaments/${id}/teams`),
    enabled: !!id,
  })
  const teams = teamsData ?? []

  const scheduledGroupIds = new Set(heats.filter((h) => h.group_id !== null).map((h) => h.group_id))
  const scheduledDivisionIds = new Set(heats.filter((h) => h.division_id !== null).map((h) => h.division_id))

  const unscheduledGroups =
    currentRound?.groups
      .filter((g) => !scheduledGroupIds.has(g.id))
      .map((g, index) => ({
        id: g.id,
        label: `${t('schedule_round_create_group_label', { number: index + 1 })} (${g.team_ids.length} teams)`,
      })) ?? []
  const unscheduledDivisions =
    currentRound?.divisions
      .filter((d) => !scheduledDivisionIds.has(d.id))
      .map((d) => ({ id: d.id, label: `${d.name} (${d.team_ids.length} teams)` })) ?? []

  return (
    <div>
      <ScheduleCourses />
      {rounds.length === 0 && <ScheduleRoundCreate />}
      {currentRound && (
        <h2 className="mb-4 text-lg font-semibold">
          {t('schedule_round_history_entry', { number: currentRound.round_number, status: currentRound.status })}
        </h2>
      )}
      {currentRound && unscheduledGroups.length > 0 && (
        <ScheduleAssignments mode="group" roundId={currentRound.id} items={unscheduledGroups} />
      )}
      {currentRound && unscheduledDivisions.length > 0 && (
        <ScheduleAssignments mode="division" roundId={currentRound.id} items={unscheduledDivisions} />
      )}
      {currentRound && (
        <ScheduleHeats heats={currentRoundHeats} courses={courses} currentRound={currentRound} teams={teams} />
      )}
      {currentRound && <ScheduleRoundActions currentRound={currentRound} allRounds={rounds} />}
    </div>
  )
}

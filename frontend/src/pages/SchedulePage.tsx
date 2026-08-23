import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Round } from '../api/types'
import { ScheduleCourses } from './ScheduleCourses'
import { ScheduleRoundCreate } from './ScheduleRoundCreate'

export function SchedulePage() {
  const { id } = useParams<{ id: string }>()

  const { data: roundsData } = useQuery({
    queryKey: ['rounds', id],
    queryFn: () => api.get<{ rounds: Round[] }>(`/api/tournaments/${id}/rounds`),
    enabled: !!id,
  })
  const rounds = roundsData?.rounds ?? []

  return (
    <div>
      <ScheduleCourses />
      {rounds.length === 0 && <ScheduleRoundCreate />}
    </div>
  )
}

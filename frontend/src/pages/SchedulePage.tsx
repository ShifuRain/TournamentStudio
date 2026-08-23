import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Round } from '../api/types'
import { ScheduleCourses } from './ScheduleCourses'

export function SchedulePage() {
  const { id } = useParams<{ id: string }>()

  useQuery({
    queryKey: ['rounds', id],
    queryFn: () => api.get<{ rounds: Round[] }>(`/api/tournaments/${id}/rounds`),
    enabled: !!id,
  })

  return (
    <div>
      <ScheduleCourses />
    </div>
  )
}

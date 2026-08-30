import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course } from '../api/types'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'

interface ScheduleAssignmentsProps {
  mode: 'group' | 'division'
  roundId: number
  items: { id: number; label: string }[]
}

export function ScheduleAssignments({ mode, roundId, items }: ScheduleAssignmentsProps) {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const { data: coursesData } = useQuery({
    queryKey: ['courses', id],
    queryFn: () => api.get<{ courses: Course[] }>(`/api/tournaments/${id}/courses`),
    enabled: !!id,
  })
  const courses = coursesData?.courses ?? []

  const [selectedCourse, setSelectedCourse] = useState<Record<number, number>>({})

  const scheduleMutation = useMutation({
    mutationFn: () => {
      const assignments = items.map((item) => ({
        [mode === 'group' ? 'group_id' : 'division_id']: item.id,
        course_id: selectedCourse[item.id],
      }))
      const path =
        mode === 'group'
          ? `/api/tournaments/${id}/rounds/${roundId}/schedule`
          : `/api/tournaments/${id}/divisions/schedule`
      return api.post(path, { assignments })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['schedule', id] })
    },
  })

  if (role !== 'organizer' || items.length === 0) {
    return null
  }

  const allCourseSelected = items.every((item) => selectedCourse[item.id] !== undefined)

  return (
    <section className={`mb-6 ${ui.panel}`}>
      <h2 className={`mb-3 ${ui.h2}`}>
        {t(mode === 'group' ? 'schedule_assignments_group_title' : 'schedule_assignments_division_title')}
      </h2>
      <ul className="mb-4 space-y-2">
        {items.map((item) => (
          <li key={item.id} className="flex items-center justify-between text-sm">
            <span>{item.label}</span>
            <label className="flex items-center gap-2 font-mono text-xs text-slate">
              {t('schedule_assignments_course_label')}
              <select
                aria-label={`${t('schedule_assignments_course_label')} — ${item.label}`}
                value={selectedCourse[item.id] ?? ''}
                onChange={(e) => setSelectedCourse((prev) => ({ ...prev, [item.id]: Number(e.target.value) }))}
                className={ui.select}
              >
                <option value="" disabled>
                  {t('schedule_assignments_select_course')}
                </option>
                {courses.map((course) => (
                  <option key={course.id} value={course.id}>
                    {course.name}
                  </option>
                ))}
              </select>
            </label>
          </li>
        ))}
      </ul>
      {scheduleMutation.isError && (
        <p role="alert" className={`mb-2 ${ui.error}`}>
          {t('schedule_assignments_error')}
        </p>
      )}
      <Button
        type="button"
        onClick={() => scheduleMutation.mutate()}
        disabled={!allCourseSelected || scheduleMutation.isPending}
        size="sm"
      >
        {t('schedule_assignments_submit')}
      </Button>
    </section>
  )
}

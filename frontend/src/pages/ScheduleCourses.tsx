import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Course } from '../api/types'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'

export function ScheduleCourses() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const { data } = useQuery({
    queryKey: ['courses', id],
    queryFn: () => api.get<{ courses: Course[] }>(`/api/tournaments/${id}/courses`),
    enabled: !!id,
  })
  const courses = data?.courses ?? []

  const [name, setName] = useState('')
  const [intervalSeconds, setIntervalSeconds] = useState(300)

  const createMutation = useMutation({
    mutationFn: () =>
      api.post<Course>(`/api/tournaments/${id}/courses`, { name, heat_interval_seconds: intervalSeconds }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['courses', id] })
      setName('')
      setIntervalSeconds(300)
    },
  })

  const updateMutation = useMutation({
    mutationFn: (vars: { courseId: number; delayOffsetSeconds: number }) =>
      api.patch<Course>(`/api/tournaments/${id}/courses/${vars.courseId}`, {
        delay_offset_seconds: vars.delayOffsetSeconds,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['courses', id] })
    },
  })

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    createMutation.mutate()
  }

  const canEdit = role === 'organizer'

  return (
    <section className={`mb-6 ${ui.panel}`}>
      <h2 className={`mb-3 ${ui.h2}`}>{t('schedule_courses_title')}</h2>
      <ul className={`mb-4 ${ui.divider}`}>
        {courses.map((course) => (
          <li key={course.id} className="flex items-center justify-between py-2 text-sm">
            <span>
              {course.name} — {course.heat_interval_seconds}s interval
            </span>
            {canEdit && (
              <label className="flex items-center gap-2 font-mono text-xs text-slate">
                {t('schedule_courses_offset')}
                <input
                  type="number"
                  defaultValue={course.delay_offset_seconds}
                  onBlur={(e) =>
                    updateMutation.mutate({ courseId: course.id, delayOffsetSeconds: Number(e.target.value) })
                  }
                  className={`w-20 ${ui.input}`}
                  aria-label={`${t('schedule_courses_offset')} — ${course.name}`}
                />
              </label>
            )}
          </li>
        ))}
      </ul>
      {updateMutation.isError && (
        <p role="alert" className={`mb-2 ${ui.error}`}>
          {t('schedule_courses_update_error')}
        </p>
      )}
      {canEdit && (
        <form onSubmit={handleSubmit} className="flex items-end gap-3">
          <div>
            <label htmlFor="course-name" className={ui.label}>
              {t('schedule_courses_name')}
            </label>
            <input id="course-name" value={name} onChange={(e) => setName(e.target.value)} className={ui.input} required />
          </div>
          <div>
            <label htmlFor="course-interval" className={ui.label}>
              {t('schedule_courses_interval')}
            </label>
            <input
              id="course-interval"
              type="number"
              min={1}
              value={intervalSeconds}
              onChange={(e) => setIntervalSeconds(Number(e.target.value))}
              className={`w-32 ${ui.input}`}
              required
            />
          </div>
          <Button type="submit" disabled={createMutation.isPending} size="sm">
            {t('schedule_courses_add_submit')}
          </Button>
        </form>
      )}
      {createMutation.isError && (
        <p role="alert" className={`mt-2 ${ui.error}`}>
          {t('schedule_courses_add_error')}
        </p>
      )}
    </section>
  )
}

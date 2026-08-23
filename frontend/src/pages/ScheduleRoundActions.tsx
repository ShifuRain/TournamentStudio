import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Round } from '../api/types'

interface ScheduleRoundActionsProps {
  currentRound: Round
  allRounds: Round[]
}

interface Cut {
  name: string
  size: number
}

export function ScheduleRoundActions({ currentRound, allRounds }: ScheduleRoundActionsProps) {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { role } = useAuth()
  const queryClient = useQueryClient()

  const nextRoundMutation = useMutation({
    mutationFn: () => api.post(`/api/tournaments/${id}/rounds/${currentRound.id}/next`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  const [cuts, setCuts] = useState<Cut[]>([{ name: '', size: 1 }])
  const divisionsMutation = useMutation({
    mutationFn: () =>
      api.post(`/api/tournaments/${id}/rounds/${currentRound.id}/divisions`, {
        cuts: cuts.filter((c) => c.name.trim() !== ''),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['rounds', id] })
    },
  })

  if (role !== 'organizer' || currentRound.status !== 'closed') {
    return (
      <RoundHistory allRounds={allRounds} />
    )
  }

  return (
    <>
      <section className="mb-6 rounded border bg-white p-4">
        <div className="mb-4">
          {nextRoundMutation.isError && (
            <p role="alert" className="mb-2 text-sm text-red-600">
              {t('schedule_round_actions_next_error')}
            </p>
          )}
          <button
            type="button"
            onClick={() => nextRoundMutation.mutate()}
            disabled={nextRoundMutation.isPending}
            className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {t('schedule_round_actions_next')}
          </button>
        </div>

        <h3 className="mb-2 text-sm font-semibold">{t('schedule_round_actions_divisions_title')}</h3>
        {cuts.map((cut, index) => (
          <div key={index} className="mb-2 flex items-end gap-2">
            <div>
              <label htmlFor={`cut-name-${index}`} className="block text-xs font-medium">
                {t('schedule_round_actions_divisions_name')}
              </label>
              <input
                id={`cut-name-${index}`}
                value={cut.name}
                onChange={(e) =>
                  setCuts((prev) => prev.map((c, i) => (i === index ? { ...c, name: e.target.value } : c)))
                }
                className="rounded border px-2 py-1 text-sm"
              />
            </div>
            <div>
              <label htmlFor={`cut-size-${index}`} className="block text-xs font-medium">
                {t('schedule_round_actions_divisions_size')}
              </label>
              <input
                id={`cut-size-${index}`}
                type="number"
                min={1}
                value={cut.size}
                onChange={(e) =>
                  setCuts((prev) =>
                    prev.map((c, i) => (i === index ? { ...c, size: Number(e.target.value) } : c)),
                  )
                }
                className="w-20 rounded border px-2 py-1 text-sm"
              />
            </div>
          </div>
        ))}
        <button
          type="button"
          onClick={() => setCuts((prev) => [...prev, { name: '', size: 1 }])}
          className="mb-3 rounded border px-3 py-1 text-xs"
        >
          {t('schedule_round_actions_divisions_add_cut')}
        </button>
        {divisionsMutation.isError && (
          <p role="alert" className="mb-2 text-sm text-red-600">
            {t('schedule_round_actions_divisions_error')}
          </p>
        )}
        <div>
          <button
            type="button"
            onClick={() => divisionsMutation.mutate()}
            disabled={divisionsMutation.isPending}
            className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {t('schedule_round_actions_divisions_submit')}
          </button>
        </div>
      </section>
      <RoundHistory allRounds={allRounds} />
    </>
  )
}

function RoundHistory({ allRounds }: { allRounds: Round[] }) {
  const { t } = useTranslation()
  const earlierRounds = allRounds.slice(0, -1)

  if (earlierRounds.length === 0) {
    return null
  }

  return (
    <section className="mb-6 rounded border bg-white p-4">
      <h2 className="mb-3 text-lg font-semibold">{t('schedule_round_history_title')}</h2>
      <ul className="space-y-1 text-sm text-gray-600">
        {earlierRounds.map((round) => (
          <li key={round.id}>
            {t('schedule_round_history_entry', { number: round.round_number, status: round.status })}
          </li>
        ))}
      </ul>
    </section>
  )
}

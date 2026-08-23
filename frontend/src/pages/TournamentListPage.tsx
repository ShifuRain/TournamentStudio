import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../auth/AuthContext'
import { api } from '../api/client'
import type { Tournament } from '../api/types'

export function TournamentListPage() {
  const { t } = useTranslation()
  const { role } = useAuth()
  const { data: tournaments, isLoading } = useQuery({
    queryKey: ['tournaments'],
    queryFn: () => api.get<Tournament[]>('/api/tournaments'),
  })

  return (
    <div className="mx-auto max-w-3xl p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-bold">{t('tournament_list_title')}</h1>
        {role === 'organizer' && (
          <Link to="/tournaments/new" className="rounded bg-blue-600 px-4 py-2 text-sm text-white">
            {t('tournament_create_link')}
          </Link>
        )}
      </div>
      {isLoading && <p>{t('loading')}</p>}
      <ul className="divide-y rounded border bg-white">
        {tournaments?.map((tournament) => (
          <li key={tournament.id}>
            <Link to={`/tournaments/${tournament.id}`} className="block px-4 py-3 hover:bg-gray-50">
              {tournament.name}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}

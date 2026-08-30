import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../auth/AuthContext'
import { api } from '../api/client'
import type { Tournament } from '../api/types'
import { ui } from '../components/ui/styles'

export function TournamentListPage() {
  const { t } = useTranslation()
  const { role } = useAuth()
  const { data: tournaments, isLoading } = useQuery({
    queryKey: ['tournaments'],
    queryFn: () => api.get<Tournament[]>('/api/tournaments'),
  })

  return (
    <div className={ui.page}>
      <div className="mb-6 flex items-center justify-between">
        <h1 className={ui.h1}>{t('tournament_list_title')}</h1>
        {role === 'organizer' && (
          <Link
            to="/tournaments/new"
            className="rounded-md bg-yellow px-4 py-2 font-display text-sm font-extrabold uppercase tracking-wide text-navy hover:bg-yellow/90"
          >
            {t('tournament_create_link')}
          </Link>
        )}
      </div>
      {isLoading && <p className={ui.muted}>{t('loading')}</p>}
      <ul className={`${ui.panel} ${ui.divider} p-0`}>
        {tournaments?.map((tournament) => (
          <li key={tournament.id}>
            <Link to={`/tournaments/${tournament.id}`} className="block px-4 py-3 hover:bg-foam/5">
              {tournament.name}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}

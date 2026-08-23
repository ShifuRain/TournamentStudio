import { NavLink, Outlet, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Tournament } from '../api/types'

export function TournamentDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: tournament, isLoading } = useQuery({
    queryKey: ['tournament', id],
    queryFn: () => api.get<Tournament>(`/api/tournaments/${id}`),
    enabled: !!id,
  })

  const tabLinkClass = ({ isActive }: { isActive: boolean }) =>
    `border-b-2 px-4 py-2 text-sm ${
      isActive ? 'border-blue-600 font-medium text-blue-600' : 'border-transparent text-gray-500'
    }`

  return (
    <div className="mx-auto max-w-3xl p-8">
      {isLoading && <p>{t('loading')}</p>}
      {tournament && (
        <>
          <h1 className="mb-1 text-xl font-bold">{tournament.name}</h1>
          <p className="mb-6 text-sm text-gray-500">{tournament.status}</p>
        </>
      )}
      <nav className="mb-6 flex border-b">
        <NavLink to="teams" className={tabLinkClass}>
          {t('tab_teams')}
        </NavLink>
        <span className="cursor-not-allowed px-4 py-2 text-sm text-gray-300" title={t('tab_coming_soon')}>
          {t('tab_schedule')}
        </span>
        <span className="cursor-not-allowed px-4 py-2 text-sm text-gray-300" title={t('tab_coming_soon')}>
          {t('tab_standings')}
        </span>
      </nav>
      <Outlet />
    </div>
  )
}

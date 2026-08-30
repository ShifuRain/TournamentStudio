import { NavLink, Outlet, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { Tournament } from '../api/types'
import { ui } from '../components/ui/styles'

export function TournamentDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data: tournament, isLoading } = useQuery({
    queryKey: ['tournament', id],
    queryFn: () => api.get<Tournament>(`/api/tournaments/${id}`),
    enabled: !!id,
  })

  const tabLinkClass = ({ isActive }: { isActive: boolean }) =>
    `border-b-2 px-4 py-2 font-mono text-xs uppercase tracking-wide ${
      isActive ? 'border-yellow text-foam' : 'border-transparent text-slate hover:text-foam-dim'
    }`

  return (
    <div className={ui.page}>
      {isLoading && <p className={ui.muted}>{t('loading')}</p>}
      {tournament && (
        <>
          <h1 className={`mb-1 ${ui.h1}`}>{tournament.name}</h1>
          <p className={`mb-6 font-mono text-xs uppercase tracking-wide ${ui.faint}`}>{tournament.status}</p>
        </>
      )}
      <nav className="mb-6 flex border-b border-hairline">
        <NavLink to="teams" className={tabLinkClass}>
          {t('tab_teams')}
        </NavLink>
        <NavLink to="schedule" className={tabLinkClass}>
          {t('tab_schedule')}
        </NavLink>
        <NavLink to="watch" className={tabLinkClass}>
          {t('tab_standings')}
        </NavLink>
      </nav>
      <Outlet />
    </div>
  )
}

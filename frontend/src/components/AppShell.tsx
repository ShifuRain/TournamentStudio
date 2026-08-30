import { Link, Outlet } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { useAvailableLanguages, changeLanguage } from '../i18n/i18n'

export function AppShell() {
  const { t, i18n } = useTranslation()
  const { role, logout } = useAuth()
  const availableLanguages = useAvailableLanguages()

  return (
    <div className="min-h-screen bg-navy bg-[radial-gradient(120%_90%_at_50%_-10%,var(--color-navy-2)_0%,var(--color-navy)_55%)] text-foam">
      <nav className="flex items-center justify-between border-b border-hairline px-6 py-3">
        <Link to="/tournaments" className="font-display text-lg font-extrabold uppercase tracking-wide text-foam">
          TournamentStudio
        </Link>
        <div className="flex items-center gap-4 font-mono text-xs">
          <Link to="/plugins" className="text-teal-tint hover:text-foam">
            {t('nav_plugins')}
          </Link>
          <select
            value={i18n.language}
            onChange={(e) => changeLanguage(e.target.value)}
            aria-label={t('nav_language')}
            className="rounded border border-hairline bg-navy-2/40 px-2 py-1 text-foam"
          >
            {availableLanguages.map((lang) => (
              <option key={lang} value={lang}>
                {lang.toUpperCase()}
              </option>
            ))}
          </select>
          {role && <span className="uppercase tracking-wide text-slate">{role}</span>}
          <button onClick={() => void logout()} className="text-red-tint hover:text-red">
            {t('nav_logout')}
          </button>
        </div>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  )
}

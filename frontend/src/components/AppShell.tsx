import { Link, Outlet } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { AVAILABLE_LANGUAGES, changeLanguage } from '../i18n/i18n'

export function AppShell() {
  const { t, i18n } = useTranslation()
  const { role, logout } = useAuth()

  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="flex items-center justify-between border-b bg-white px-6 py-3">
        <Link to="/tournaments" className="font-bold">
          TournamentStudio
        </Link>
        <div className="flex items-center gap-4 text-sm">
          <select
            value={i18n.language}
            onChange={(e) => changeLanguage(e.target.value)}
            aria-label={t('nav_language')}
          >
            {AVAILABLE_LANGUAGES.map((lang) => (
              <option key={lang} value={lang}>
                {lang.toUpperCase()}
              </option>
            ))}
          </select>
          {role && <span className="text-gray-500">{role}</span>}
          <button onClick={() => void logout()} className="text-blue-600">
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

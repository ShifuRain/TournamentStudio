import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { ApiError } from '../api/client'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'

export function LoginPage() {
  const { t } = useTranslation()
  const { login } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(username, password)
      navigate('/tournaments')
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError(t('login_invalid_credentials'))
      } else {
        setError(t('login_generic_error'))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-navy bg-[radial-gradient(120%_90%_at_50%_-10%,var(--color-navy-2)_0%,var(--color-navy)_55%)] px-4">
      <form onSubmit={handleSubmit} className={`w-full max-w-sm space-y-4 ${ui.panel}`}>
        <h1 className={ui.h1}>{t('login_title')}</h1>
        {error && (
          <p role="alert" className={ui.error}>
            {error}
          </p>
        )}
        <div>
          <label htmlFor="username" className={ui.label}>
            {t('login_username')}
          </label>
          <input
            id="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className={ui.input}
            required
          />
        </div>
        <div>
          <label htmlFor="password" className={ui.label}>
            {t('login_password')}
          </label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className={ui.input}
            required
          />
        </div>
        <Button type="submit" disabled={submitting} className="w-full">
          {t('login_submit')}
        </Button>
      </form>
    </div>
  )
}

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { createElement } from 'react'

describe('i18n', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        const lang = url.split('/').pop()
        const data = lang === 'de' ? { greeting: 'Hallo' } : { greeting: 'Hello' }
        return new Response(JSON.stringify(data), { status: 200 })
      }),
    )
  })

  it('loads translations from the API and resolves a key', async () => {
    const { default: i18n } = await import('./i18n')
    await i18n.init
    await new Promise((resolve) => i18n.on('loaded', resolve))
    expect(i18n.t('greeting')).toBe('Hello')
  })

  it('changeLanguage persists the choice and switches languages', async () => {
    const { default: i18n, changeLanguage } = await import('./i18n')
    await new Promise((resolve) => i18n.on('loaded', resolve))

    changeLanguage('de')
    await new Promise((resolve) => i18n.on('languageChanged', resolve))

    expect(localStorage.getItem('ts_language')).toBe('de')
    expect(i18n.t('greeting')).toBe('Hallo')
  })

  it('AVAILABLE_LANGUAGES starts out as the hardcoded fallback', async () => {
    const { AVAILABLE_LANGUAGES } = await import('./i18n')
    expect(AVAILABLE_LANGUAGES).toEqual(['en', 'de'])
  })

  it('refreshAvailableLanguages fetches languages from GET /api/i18n and updates AVAILABLE_LANGUAGES in place', async () => {
    const { AVAILABLE_LANGUAGES, refreshAvailableLanguages } = await import('./i18n')

    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url === '/api/i18n') {
          return new Response(JSON.stringify({ languages: ['en', 'de', 'fr'] }), { status: 200 })
        }
        return new Response(JSON.stringify({ greeting: 'Hello' }), { status: 200 })
      }),
    )

    await refreshAvailableLanguages()

    expect(AVAILABLE_LANGUAGES).toEqual(['en', 'de', 'fr'])
  })

  it('refreshAvailableLanguages keeps the hardcoded fallback if the request fails', async () => {
    const { AVAILABLE_LANGUAGES, refreshAvailableLanguages } = await import('./i18n')

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('', { status: 500 })),
    )

    await refreshAvailableLanguages()

    expect(AVAILABLE_LANGUAGES).toEqual(['en', 'de'])
  })

  it('refreshAvailableLanguages keeps the hardcoded fallback if fetch throws', async () => {
    const { AVAILABLE_LANGUAGES, refreshAvailableLanguages } = await import('./i18n')

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network error')
      }),
    )

    await refreshAvailableLanguages()

    expect(AVAILABLE_LANGUAGES).toEqual(['en', 'de'])
  })

  it('useAvailableLanguages re-renders a component once refreshAvailableLanguages resolves with new languages', async () => {
    const { useAvailableLanguages, refreshAvailableLanguages } = await import('./i18n')

    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url === '/api/i18n') {
          return new Response(JSON.stringify({ languages: ['en', 'de', 'fr'] }), { status: 200 })
        }
        return new Response(JSON.stringify({ greeting: 'Hello' }), { status: 200 })
      }),
    )

    function Probe() {
      const languages = useAvailableLanguages()
      return createElement('span', null, languages.join(','))
    }

    render(createElement(Probe))
    expect(screen.getByText('en,de')).toBeInTheDocument()

    await refreshAvailableLanguages()

    await waitFor(() => {
      expect(screen.getByText('en,de,fr')).toBeInTheDocument()
    })
  })
})

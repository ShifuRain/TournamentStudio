import { beforeEach, describe, expect, it, vi } from 'vitest'

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
})

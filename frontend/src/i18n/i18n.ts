import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { useSyncExternalStore } from 'react'
import { apiBackend } from './backend'

const LANGUAGE_STORAGE_KEY = 'ts_language'

// Hardcoded fallback for language detection before GET /api/i18n has
// resolved (or if it fails, e.g. offline or an older server) -- kept in
// sync with the languages bundled in internal/i18n/bundles.
const FALLBACK_LANGUAGES = ['en', 'de']

// The deployment's actual available languages: the bundled ones plus
// any drop-in language added via TOURNAMENTSTUDIO_LANGUAGES
// (internal/i18n.Catalog.Languages()). Starts out as the hardcoded
// fallback and is mutated in place by refreshAvailableLanguages below
// once GET /api/i18n resolves, so existing `import { AVAILABLE_LANGUAGES }`
// references (e.g. the language picker in TournamentCreatePage) keep
// seeing the same array reference.
export const AVAILABLE_LANGUAGES: string[] = [...FALLBACK_LANGUAGES]

// useAvailableLanguages (below) needs a snapshot that changes reference
// whenever AVAILABLE_LANGUAGES' contents change -- useSyncExternalStore
// compares snapshots with Object.is, so mutating AVAILABLE_LANGUAGES in
// place (same reference) would never be seen as a change. This cache is
// rebuilt only when refreshAvailableLanguages actually updates the list.
let languagesSnapshot: readonly string[] = AVAILABLE_LANGUAGES
const languageListeners = new Set<() => void>()

function subscribeAvailableLanguages(listener: () => void): () => void {
  languageListeners.add(listener)
  return () => languageListeners.delete(listener)
}

function getAvailableLanguagesSnapshot(): readonly string[] {
  return languagesSnapshot
}

// React hook for components that render the language list: re-renders
// automatically once GET /api/i18n resolves and reveals drop-in languages,
// unlike reading AVAILABLE_LANGUAGES directly (which is correct data but
// triggers no re-render on mutation).
export function useAvailableLanguages(): readonly string[] {
  return useSyncExternalStore(subscribeAvailableLanguages, getAvailableLanguagesSnapshot)
}

// Fetches the deployment's available languages from the backend and
// updates AVAILABLE_LANGUAGES in place. Exported so callers/tests can
// await it directly; also fired once, unawaited, at module init below.
export async function refreshAvailableLanguages(): Promise<void> {
  try {
    const res = await fetch('/api/i18n')
    if (!res.ok) {
      return
    }
    const data = (await res.json()) as { languages?: string[] }
    if (Array.isArray(data.languages) && data.languages.length > 0) {
      AVAILABLE_LANGUAGES.splice(0, AVAILABLE_LANGUAGES.length, ...data.languages)
      languagesSnapshot = [...AVAILABLE_LANGUAGES]
      languageListeners.forEach((listener) => listener())
    }
  } catch {
    // Network error or bad response -- keep the hardcoded fallback.
  }
}

void refreshAvailableLanguages()

function detectLanguage(): string {
  const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY)
  if (stored && AVAILABLE_LANGUAGES.includes(stored)) {
    return stored
  }
  const browserLang = navigator.language.split('-')[0]
  return AVAILABLE_LANGUAGES.includes(browserLang) ? browserLang : 'en'
}

void i18n
  .use(apiBackend)
  .use(initReactI18next)
  .init({
    lng: detectLanguage(),
    fallbackLng: 'en',
    interpolation: { escapeValue: false },
  })

export function changeLanguage(lang: string): void {
  localStorage.setItem(LANGUAGE_STORAGE_KEY, lang)
  void i18n.changeLanguage(lang)
}

export default i18n

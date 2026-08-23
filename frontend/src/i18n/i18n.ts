import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { apiBackend } from './backend'

const LANGUAGE_STORAGE_KEY = 'ts_language'

export const AVAILABLE_LANGUAGES = ['en', 'de']

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

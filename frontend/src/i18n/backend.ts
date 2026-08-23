import type { BackendModule, ReadCallback } from 'i18next'

// A minimal i18next backend that fetches one language's whole flat
// key->string map from GET /api/i18n/{lang} in a single request,
// instead of i18next's usual per-namespace file convention -- this
// project has exactly one namespace (the whole catalog), matching the
// backend's own shape (internal/i18n.Catalog.Strings).
export const apiBackend: BackendModule = {
  type: 'backend',
  init: () => {},
  read: (language: string, _namespace: string, callback: ReadCallback) => {
    fetch(`/api/i18n/${language}`)
      .then((res) => {
        if (!res.ok) {
          throw new Error(`failed to load translations for ${language}: ${res.status}`)
        }
        return res.json()
      })
      .then((data) => callback(null, data))
      .catch((err) => callback(err, null))
  },
}

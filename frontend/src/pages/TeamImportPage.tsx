import { useState, type ChangeEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { ImportResult } from '../api/types'

export function TeamImportPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const [file, setFile] = useState<File | null>(null)

  const importMutation = useMutation({
    mutationFn: () => {
      const form = new FormData()
      form.append('file', file as File)
      return api.postForm<ImportResult>(`/api/tournaments/${id}/teams/import`, form)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['teams', id] })
    },
  })

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    setFile(e.target.files?.[0] ?? null)
  }

  function handleSubmit() {
    if (file) {
      importMutation.mutate()
    }
  }

  return (
    <div className="mx-auto max-w-lg p-8">
      <h1 className="mb-6 text-xl font-bold">{t('import_title')}</h1>

      {!importMutation.data && (
        <div className="space-y-4">
          <input type="file" accept=".csv,.xlsx" aria-label={t('import_file_label')} onChange={handleFileChange} />
          <div>
            <button
              onClick={handleSubmit}
              disabled={!file || importMutation.isPending}
              className="rounded bg-blue-600 px-4 py-2 text-sm text-white disabled:opacity-50"
            >
              {t('import_submit')}
            </button>
          </div>
          {importMutation.isError && (
            <p role="alert" className="text-sm text-red-600">
              {t('import_error')}
            </p>
          )}
        </div>
      )}

      {importMutation.data && (
        <div>
          <p className="mb-4">{t('import_result_summary', { count: importMutation.data.imported })}</p>
          {importMutation.data.problems.length > 0 && (
            <ul className="mb-4 list-disc space-y-1 pl-5 text-sm text-red-600">
              {importMutation.data.problems.map((problem, i) => (
                <li key={i}>{t('import_row_problem', { row: problem.row_index, message: problem.message })}</li>
              ))}
            </ul>
          )}
          <Link to={`/tournaments/${id}/teams`} className="text-blue-600">
            {t('import_back_link')}
          </Link>
        </div>
      )}
    </div>
  )
}

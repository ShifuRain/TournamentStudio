import { useState, type ChangeEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { ImportResult } from '../api/types'
import { ui } from '../components/ui/styles'
import { Button } from '../components/ui/Button'

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
    <div className={ui.pageNarrow}>
      <h1 className={`mb-6 ${ui.h1}`}>{t('import_title')}</h1>

      {!importMutation.data && (
        <div className={`space-y-4 ${ui.panel}`}>
          <input
            type="file"
            accept=".csv,.xlsx"
            aria-label={t('import_file_label')}
            onChange={handleFileChange}
            className="font-mono text-sm text-foam-dim file:mr-3 file:rounded file:border file:border-hairline file:bg-navy-2/40 file:px-3 file:py-1.5 file:text-foam"
          />
          <div>
            <Button onClick={handleSubmit} disabled={!file || importMutation.isPending} size="sm">
              {t('import_submit')}
            </Button>
          </div>
          {importMutation.isError && (
            <p role="alert" className={ui.error}>
              {t('import_error')}
            </p>
          )}
        </div>
      )}

      {importMutation.data && (
        <div className={ui.panel}>
          <p className={`mb-4 ${ui.muted}`}>{t('import_result_summary', { count: importMutation.data.imported })}</p>
          {importMutation.data.problems.length > 0 && (
            <ul className="mb-4 list-disc space-y-1 pl-5 text-sm text-red-tint">
              {importMutation.data.problems.map((problem, i) => (
                <li key={i}>{t('import_row_problem', { row: problem.row_index, message: problem.message })}</li>
              ))}
            </ul>
          )}
          <Link to={`/tournaments/${id}/teams`} className={ui.link}>
            {t('import_back_link')}
          </Link>
        </div>
      )}
    </div>
  )
}

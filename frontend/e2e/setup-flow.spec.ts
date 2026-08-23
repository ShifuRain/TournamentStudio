import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('login, create a tournament, add a team, and import a CSV with one bad row', async ({ page }) => {
  await page.goto('/login')

  await page.getByLabel('Username').fill('organizer1')
  await page.getByLabel('Password').fill('e2e-test-password')
  await page.getByRole('button', { name: 'Sign In' }).click()

  await expect(page).toHaveURL(/\/tournaments$/)

  await page.getByRole('link', { name: 'Create Tournament' }).click()
  await expect(page).toHaveURL(/\/tournaments\/new$/)

  await page.getByLabel('Name', { exact: true }).fill('E2E Regatta')
  await page.getByLabel('Sport').selectOption({ label: 'Dragonboat' })
  await page.getByLabel('Tournament Type').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Create' }).click()

  await expect(page).toHaveURL(/\/tournaments\/\d+\/teams$/)
  await expect(page.getByRole('heading', { name: 'E2E Regatta' })).toBeVisible()

  await page.getByLabel('Name', { exact: true }).fill('Rhein Dragons')
  await page.getByRole('button', { name: 'Add Team' }).click()
  await expect(page.getByText('Rhein Dragons')).toBeVisible()

  await page.getByRole('link', { name: 'Import from file' }).click()
  await expect(page).toHaveURL(/\/teams\/import$/)

  const fixturePath = path.join(__dirname, 'fixtures', 'teams.csv')
  await page.getByLabel('Choose a CSV or XLSX file').setInputFiles(fixturePath)
  await page.getByRole('button', { name: 'Upload' }).click()

  await expect(page.getByText(/1 team\(s\) imported\./)).toBeVisible()
  await expect(page.getByText(/Row 1: missing team name/)).toBeVisible()
})

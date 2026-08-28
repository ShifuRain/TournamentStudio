import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('organizer uploads a plugin and it becomes selectable when creating a tournament', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username').fill('organizer1')
  await page.getByLabel('Password').fill('e2e-test-password')
  await page.getByRole('button', { name: 'Sign In' }).click()
  await expect(page).toHaveURL(/\/tournaments$/)

  await page.getByRole('link', { name: 'Plugins' }).click()
  await expect(page).toHaveURL(/\/plugins$/)
  await expect(page.getByText('Dragonboat')).toBeVisible()

  const fixturePath = path.join(__dirname, 'fixtures', 'extra-sport.lua')
  await page.getByLabel('Choose a .lua file').setInputFiles(fixturePath)
  await page.getByRole('button', { name: 'Upload' }).click()
  await expect(page.getByText('E2E Extra Sport')).toBeVisible()

  // "Create Tournament" only appears on the tournaments list page, not on
  // /plugins -- navigate back there via the brand link before following it.
  await page.getByRole('link', { name: 'TournamentStudio' }).click()
  await expect(page).toHaveURL(/\/tournaments$/)
  await page.getByRole('link', { name: 'Create Tournament' }).click()
  await expect(page.getByLabel('Sport')).toContainText('E2E Extra Sport')
})

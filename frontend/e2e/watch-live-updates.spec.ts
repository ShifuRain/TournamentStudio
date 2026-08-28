import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('watch page reflects a result submitted elsewhere, without a manual reload', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username').fill('organizer1')
  await page.getByLabel('Password').fill('e2e-test-password')
  await page.getByRole('button', { name: 'Sign In' }).click()
  await expect(page).toHaveURL(/\/tournaments$/)

  await page.getByRole('link', { name: 'Create Tournament' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Watch Test Cup')
  await page.getByLabel('Sport').selectOption({ label: 'Dragonboat' })
  await page.getByLabel('Tournament Type').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Create' }).click()

  await expect(page).toHaveURL(/\/tournaments\/\d+\/teams$/)
  const tournamentIdMatch = page.url().match(/\/tournaments\/(\d+)\//)
  if (!tournamentIdMatch) throw new Error(`could not extract tournament id from URL: ${page.url()}`)
  const tournamentId = tournamentIdMatch[1]

  await page.getByRole('link', { name: 'Import from file' }).click()
  const fixturePath = path.join(__dirname, 'fixtures', 'teams.csv')
  await page.getByLabel('Choose a CSV or XLSX file').setInputFiles(fixturePath)
  await page.getByRole('button', { name: 'Upload' }).click()
  await expect(page.getByText(/4 team\(s\) imported\./)).toBeVisible()

  await page.getByRole('link', { name: 'Rounds & Schedule' }).click()
  await page.getByLabel(/^Name$/).fill('Lane 1')
  await page.getByLabel(/heat interval/i).fill('60')
  await page.getByRole('button', { name: 'Add Course' }).click()
  await expect(page.getByText(/Lane 1/)).toBeVisible()

  await page.getByLabel(/number of groups/i).fill('2')
  await page.getByRole('button', { name: 'Shuffle into groups' }).click()
  await page.getByRole('button', { name: 'Create Round 1' }).click()
  await expect(page.getByText("Schedule this round's groups")).toBeVisible()

  const courseSelects = page.locator('select[aria-label*="Course —"]')
  await expect(courseSelects).toHaveCount(2)
  for (let i = 0; i < 2; i++) {
    await courseSelects.nth(i).selectOption({ label: 'Lane 1' })
  }
  await page.getByRole('button', { name: 'Schedule' }).click()
  await expect(page.getByRole('heading', { name: 'Heats' })).toBeVisible()

  // Grab one heat's id directly from the backend so the result can be
  // submitted via the API (not the UI) -- proving the Watch page picks up
  // a change made by someone else, not just its own writes.
  const token = await page.evaluate(() => localStorage.getItem('ts_token'))
  const scheduleRes = await page.request.get(`/api/tournaments/${tournamentId}/schedule`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(scheduleRes.ok()).toBe(true)
  const scheduleBody = (await scheduleRes.json()) as { heats: { id: number; group_id: number | null }[] }
  const heat = scheduleBody.heats.find((h) => h.group_id !== null)
  if (!heat) throw new Error('expected at least one scheduled group heat')

  const roundsRes = await page.request.get(`/api/tournaments/${tournamentId}/rounds`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const roundsBody = (await roundsRes.json()) as { rounds: { groups: { id: number; team_ids: string[] }[] }[] }
  const group = roundsBody.rounds[0].groups.find((g) => g.id === heat.group_id)
  if (!group) throw new Error('expected the heat\'s group to exist')
  const [firstTeamId] = group.team_ids

  // Now navigate to the Watch tab and leave it untouched. Nobody has a
  // result yet, so the round renders with no ranked-team rows at all
  // (unranked teams are omitted entirely -- see the standings endpoint's
  // design) -- confirm the page loaded via its round heading only.
  await page.getByRole('link', { name: 'Live Standings' }).click()
  await expect(page).toHaveURL(/\/watch$/)
  await expect(page.getByText('Round 1 — open')).toBeVisible()
  await expect(page.getByText('123.45s')).toHaveCount(0)

  // Submit a result for that heat via the raw API -- simulating a second
  // operator elsewhere, not this page's own action.
  const submitRes = await page.request.post(`/api/tournaments/${tournamentId}/heats/${heat.id}/results`, {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: { [firstTeamId]: { time_seconds: 123.45 } },
  })
  expect(submitRes.ok()).toBe(true)

  // The Watch page must reflect this without any reload or click here.
  await expect(page.getByText('123.45s')).toBeVisible({ timeout: 10_000 })
})

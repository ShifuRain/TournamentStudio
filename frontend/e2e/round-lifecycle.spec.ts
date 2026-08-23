import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

test('organizer runs a full round: create, schedule, results, next round', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username').fill('organizer1')
  await page.getByLabel('Password').fill('e2e-test-password')
  await page.getByRole('button', { name: 'Sign In' }).click()

  await expect(page).toHaveURL(/\/tournaments$/)

  await page.getByRole('link', { name: 'Create Tournament' }).click()
  await expect(page).toHaveURL(/\/tournaments\/new$/)

  await page.getByLabel('Name', { exact: true }).fill('Lifecycle Test Cup')
  await page.getByLabel('Sport').selectOption({ label: 'Dragonboat' })
  await page.getByLabel('Tournament Type').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Create' }).click()

  await expect(page).toHaveURL(/\/tournaments\/\d+\/teams$/)
  const tournamentIdMatch = page.url().match(/\/tournaments\/(\d+)\//)
  if (!tournamentIdMatch) {
    throw new Error(`could not extract tournament id from URL: ${page.url()}`)
  }
  const tournamentId = tournamentIdMatch[1]

  // Import 4 valid teams (plus one deliberately bad row, per the shared
  // fixture) so round 1 can be split into two groups of two.
  await page.getByRole('link', { name: 'Import from file' }).click()
  await expect(page).toHaveURL(/\/teams\/import$/)
  const fixturePath = path.join(__dirname, 'fixtures', 'teams.csv')
  await page.getByLabel('Choose a CSV or XLSX file').setInputFiles(fixturePath)
  await page.getByRole('button', { name: 'Upload' }).click()
  await expect(page.getByText(/4 team\(s\) imported\./)).toBeVisible()

  await page.getByRole('link', { name: 'Rounds & Schedule' }).click()
  await expect(page).toHaveURL(/\/schedule$/)

  // Courses: add one course to schedule heats onto.
  await page.getByLabel(/^Name$/).fill('Lane 1')
  await page.getByLabel(/heat interval/i).fill('60')
  await page.getByRole('button', { name: 'Add Course' }).click()
  await expect(page.getByText(/Lane 1/)).toBeVisible()

  // Round 1: shuffle the 4 imported teams into 2 groups of 2 and create.
  await page.getByLabel(/number of groups/i).fill('2')
  await page.getByRole('button', { name: 'Shuffle into groups' }).click()
  await page.getByRole('button', { name: 'Create Round 1' }).click()
  await expect(page.getByText("Schedule this round's groups")).toBeVisible()

  // Schedule both groups onto the one course.
  const courseSelects = page.locator('select[aria-label*="Course —"]')
  await expect(courseSelects).toHaveCount(2)
  for (let i = 0; i < 2; i++) {
    await courseSelects.nth(i).selectOption({ label: 'Lane 1' })
  }
  await page.getByRole('button', { name: 'Schedule' }).click()
  await expect(page.getByRole('heading', { name: 'Heats' })).toBeVisible()

  // Submit results for every heat. Submit one heat at a time, waiting for
  // its "Submit Results" button to disappear (heat closed) before moving
  // on to the next, so the loop doesn't race the query invalidation.
  const timeInputs = page.locator('input[placeholder="Time (seconds)"]')
  await expect(timeInputs).toHaveCount(4)
  const timeCount = await timeInputs.count()
  for (let i = 0; i < timeCount; i++) {
    await timeInputs.nth(i).fill(String(100 + i * 5))
  }

  const submitButtons = page.getByRole('button', { name: 'Submit Results' })
  const submitCount = await submitButtons.count()
  for (let i = 0; i < submitCount; i++) {
    await submitButtons.first().click()
    await expect(submitButtons).toHaveCount(submitCount - i - 1)
  }

  // All heats closed -> round 1 auto-closes and offers "Create Next Round".
  const nextRoundButton = page.getByRole('button', { name: 'Create Next Round' })
  await expect(nextRoundButton).toBeVisible({ timeout: 10_000 })
  await nextRoundButton.click()

  // Round history now shows round 1 as closed, and the newly reseeded
  // round 2's groups are awaiting scheduling -- both prove a real round 2
  // was created client-side.
  await expect(page.getByText('Round history')).toBeVisible()
  await expect(page.getByText('Round 1 — closed')).toBeVisible()
  await expect(page.getByText("Schedule this round's groups")).toBeVisible()

  // Confirm directly against the real backend (not just the UI) that
  // reseeding actually ran: round 2 exists and its groups cover all 4
  // teams that played round 1.
  const token = await page.evaluate(() => localStorage.getItem('ts_token'))
  const roundsRes = await page.request.get(`/api/tournaments/${tournamentId}/rounds`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(roundsRes.ok()).toBe(true)
  const roundsBody = (await roundsRes.json()) as {
    rounds: { round_number: number; status: string; groups: { team_ids: string[] }[] }[]
  }
  expect(roundsBody.rounds).toHaveLength(2)
  expect(roundsBody.rounds[0].status).toBe('closed')
  const round2 = roundsBody.rounds[1]
  expect(round2.round_number).toBe(2)
  const round2TeamIds = round2.groups.flatMap((g) => g.team_ids)
  expect(round2TeamIds).toHaveLength(4)
  expect(new Set(round2TeamIds).size).toBe(4)
})

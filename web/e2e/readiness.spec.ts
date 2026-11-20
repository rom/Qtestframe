// Journey 5: a test that may not be executed yet. The runner explains why
// and disables the executing marks (default execution settings: only tests
// with status "ready" may be executed).
import { expect, items, test } from './helpers'

test('a draft test in a run shows why it cannot be executed and disables Pass', async ({ page, api, project }) => {
  const tc = await api.post(`/projects/${project.id}/tests`, {
    title: 'Refund a cancelled order',
    execution_type: 'manual',
    status: 'draft',
    steps: [{ action: 'Cancel a paid order', expected: 'The payment is refunded' }],
  })
  const run = await api.post(`/projects/${project.id}/runs`, { name: 'Draft check', test_ids: [tc.id] })
  const [ex] = items(await api.get(`/runs/${run.id}/executions`))
  expect(ex.readiness.allowed).toBe(false)

  await page.goto(`/p/${project.id}/runs/${run.id}/exec/${ex.id}`)
  const banner = page.locator('.readiness-banner')
  await expect(banner).toBeVisible()
  await expect(banner).toContainText('This test cannot be executed yet: 1 reason')
  await expect(banner).toContainText(`${tc.key} has status Draft; only tests with status Ready can be executed.`)
  await expect(banner.getByRole('link', { name: tc.key })).toBeVisible()

  // executing marks are disabled; Blocked and Skip stay possible
  await expect(page.getByRole('button', { name: 'Pass (P)' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Fail (F)' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Blocked (B)' })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Skip (S)' })).toBeEnabled()

  // the shortcut does not record a pass either
  await page.getByRole('heading', { name: 'Steps' }).click()
  await page.keyboard.press('p')
  await expect(page.getByRole('button', { name: 'Pass (P)' })).toHaveAttribute('aria-pressed', 'false')
  const after = await api.get(`/executions/${ex.id}`)
  expect(after.status).toBe('not_run')
})

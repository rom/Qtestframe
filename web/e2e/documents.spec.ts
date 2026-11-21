// Generated documents: create a test plan from the document archive and find
// it in the archive, highlighted, with download and open actions.
import { expect, items, settled, test } from './helpers'

test('creates a test plan and shows it in the archive', async ({ page, api, project }) => {
  const req = await api.post(`/projects/${project.id}/requirements`, { title: 'Pay by card', description: 'The shop MUST accept cards.', status: 'approved' })
  const tc = await api.post(`/projects/${project.id}/tests`, {
    title: 'Card payment',
    status: 'ready',
    steps: [{ action: 'Pay with a test card', expected: 'The order is paid' }],
  })
  await api.post(`/projects/${project.id}/links`, { requirement_id: req.id, test_case_id: tc.id })

  await page.goto(`/p/${project.id}/documents`)
  await settled(page)
  await page.getByRole('button', { name: 'Create document' }).click()
  await page.getByRole('menuitem', { name: 'Test plan' }).click()

  const dialog = page.getByRole('dialog', { name: 'Create document' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Test plan', pressed: true })).toBeVisible()
  await expect(dialog.getByText('Show for each test')).toBeVisible()
  await dialog.getByRole('button', { name: 'Markdown' }).click()
  await expect(dialog.getByRole('button', { name: 'Markdown', pressed: true })).toBeVisible()
  await dialog.getByLabel('Latest results').check()
  await dialog.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(dialog).toBeHidden()

  const name = `${project.key}-test-plan-v1.0.md`
  const banner = page.locator('.created-banner')
  await expect(banner).toContainText('New document in the archive')
  await expect(banner).toContainText(name)
  await expect(banner.getByRole('link', { name: /Download/ })).toHaveAttribute('href', /\/files\/.+\/content$/)
  const row = page.locator('tr.row-new')
  await expect(row).toContainText(name)
  await expect(row).toContainText('Document')

  // the file is in the archive with its content
  const [file] = items(await api.get(`/projects/${project.id}/documents?q=tag:test-plan`))
  expect(file.name).toBe(name)
  expect(file.tags).toEqual(['generated', 'test-plan', 'v1.0'])
  const res = await api.request.get(`/api/v1/files/${file.id}/content`)
  const md = await res.text()
  expect(md).toContain('# Test plan')
  expect(md).toMatch(new RegExp(`#{3,4} ${tc.key} Card payment`))
  expect(md).toContain('**Latest result:** Not run')
  expect(md).toMatch(new RegExp(`${req.key}.*\\| Pay by card \\|`)) // keys link to the items
})

test('the Reports page opens the generator in the archive', async ({ page, project }) => {
  await page.goto(`/p/${project.id}/reports`)
  await page.getByRole('link', { name: 'Create…' }).first().click()
  const dialog = page.getByRole('dialog', { name: 'Create document' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Requirements specification', pressed: true })).toBeVisible()
  await expect(dialog.getByLabel('Group the requirements by')).toBeVisible()
  await dialog.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.locator('.created-banner')).toContainText(`${project.key}-requirements-specification-v1.0.pdf`)
  await expect(page.locator('.created-banner').getByRole('button', { name: 'Open' })).toBeVisible()
})

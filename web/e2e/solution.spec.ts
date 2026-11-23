// Solution prompts: the solution preamble of a software project is filled
// in from a template and edited (a pasted connection string with a password
// is flagged), then an architecture prompt is built from it and copied.
import { expect, test } from './helpers'

test.use({ permissions: ['clipboard-read', 'clipboard-write'] })

test('fill the solution preamble, build an architecture prompt and copy it', async ({ page, api, project }) => {
  const base = `/projects/${project.id}`
  await api.patch(base, { type: 'software' })
  const folder = await api.post(`${base}/folders`, { name: 'Payments', kind: 'requirement' })
  await api.post(`${base}/requirements`, { title: 'Pay by card', description: 'The customer MUST be able to pay by card.', folder_id: folder.id, create_test: false })

  // the preamble: a template, then an own description
  await page.goto(`/p/${project.id}/settings?tab=solution`)
  const form = page.locator('.solution-preamble form')
  await expect(form.getByLabel('Implementation languages and versions')).toBeVisible()
  await form.locator('#sp-template').click()
  await page.getByRole('option', { name: 'Python FastAPI + PostgreSQL' }).click()
  await form.getByRole('button', { name: 'Use template' }).click()
  await expect(form.getByLabel('Implementation languages and versions')).toHaveValue(/Python 3\.12/)
  // a password in a connection string is flagged while typing
  const db = form.getByLabel('Databases (description, no credentials)')
  await db.fill('postgres://shop:' + 'Not-a-real-0ne' + '@db/shop')
  await expect(page.locator('.sp-warnings')).toContainText('A URL or connection string with a password')
  await db.fill('PostgreSQL 16 owned by the shop service; migrations with Alembic; credentials from the vault')
  await expect(page.locator('.sp-warnings')).toHaveCount(0)
  await form.getByLabel('Out of scope').fill('Gift cards')
  await form.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Solution preamble saved')).toBeVisible()
  const saved = await api.get(`${base}/solution-preamble`)
  expect(saved.preamble.version).toBe(1)
  expect(saved.preamble.out_of_scope).toBe('Gift cards')
  expect(saved.warnings).toEqual([])

  // an architecture prompt from it
  await page.goto(`/p/${project.id}/prompts?kind=architecture`)
  await expect(page.locator('.preamble-status')).toContainText('fields filled in (version 1)')
  await page.getByRole('button', { name: 'Build prompt' }).click()
  const output = page.locator('textarea.prompt-output')
  await expect(output).toHaveValue(/# Architecture of /)
  await expect(output).toHaveValue(/\*\*Implementation languages and versions:\*\* Python 3\.12/)
  await expect(output).toHaveValue(/\*\*Out of scope:\*\* Gift cards/)
  await expect(output).toHaveValue(/Pay by card/)
  await expect(page.getByText(/about \d+ tokens/)).toBeVisible()
  await page.getByRole('button', { name: 'Copy', exact: true }).click()
  await expect(page.getByText('Prompt copied')).toBeVisible()
  const copied = await page.evaluate(() => navigator.clipboard.readText())
  expect(copied).toContain('## Solution preamble')
  expect(copied).toContain('| Requirement | Covered by')
})

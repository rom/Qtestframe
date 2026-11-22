// Variants and code: marking a requirement as applying to one variant,
// coverage per variant in the matrix and the lists, "verified in build" from
// a CI upload, and a regression run made from the changed requirement.
import { expect, settled, test } from './helpers'

test('applies a requirement to a variant, shows its coverage per variant and builds a regression run', async ({ page, api, project }) => {
  const pp = `/projects/${project.id}`
  await api.post(`${pp}/variants`, { name: 'EU', kind: 'market', attributes: { region: 'EU' } })
  const us = await api.post<{ id: string }>(`${pp}/variants`, { name: 'US', kind: 'market', attributes: { region: 'US' } })
  const req = await api.post<{ id: string; key: string }>(`${pp}/requirements`, { title: 'UL listing', description: 'The device MUST be UL listed.', status: 'approved', create_test: false })
  const other = await api.post<{ id: string; key: string }>(`${pp}/requirements`, { title: 'Power on', create_test: false })
  const tc = await api.post<{ id: string; key: string }>(`${pp}/tests`, { title: 'UL check', execution_type: 'automated', status: 'ready' })
  await api.post(`${pp}/links`, { requirement_id: req.id, test_case_id: tc.id })

  // mark the requirement as applying to the US only
  await page.goto(`/p/${project.id}/requirements?item=${req.id}`)
  await settled(page)
  const drawer = page.locator('.p-drawer')
  const section = drawer.locator('section.applicability')
  await expect(section).toContainText('All variants')
  await section.getByRole('button', { name: 'Change' }).click()
  const dialog = page.getByRole('dialog', { name: 'Applies to' })
  await dialog.getByRole('button', { name: 'Selected variants' }).click()
  await dialog.locator('.p-multiselect').click()
  await page.getByRole('option', { name: 'US' }).click()
  await page.keyboard.press('Escape')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()
  await expect(section).toContainText('US')

  // CI uploads results for a build of the US variant
  const run = await api.post<{ id: string }>(`${pp}/runs`, { name: 'CI 7', test_ids: [tc.id], variant_id: us.id })
  await api.post(`/runs/${run.id}/results?build_id=7&build_number=1.0.7&branch=main&commit=abcdef1234`, { results: [{ test_key: tc.key, status: 'passed' }] })

  // the coverage matrix: passed in the US, not applicable in the EU
  await page.goto(`/p/${project.id}/variants`)
  const row = page.getByRole('row', { name: new RegExp(req.key) })
  await expect(row).toContainText('Passed')
  await expect(row).toContainText('Not applicable')

  // the list for the EU leaves it out
  await page.goto(`/p/${project.id}/requirements?variant=EU`)
  await settled(page)
  await expect(page.getByRole('row', { name: new RegExp(other.key) })).toBeVisible()
  await expect(page.getByRole('row', { name: new RegExp(req.key) })).toHaveCount(0)

  // verified in build 1.0.7
  await page.goto(`/p/${project.id}/requirements?item=${req.id}`)
  await settled(page)
  await page.locator('.p-drawer [role="tab"]:has(.pi-code)').click()
  await expect(page.locator('.code-panel')).toContainText('Verified in build')
  await expect(page.locator('.code-panel')).toContainText('1.0.7')

  // regression selection from the changed requirement, and a run of it
  await page.goto(`/p/${project.id}/code`)
  await page.getByLabel('Changed requirements').fill(req.key)
  await page.getByRole('button', { name: 'Propose tests' }).click()
  await expect(page.getByRole('row', { name: new RegExp(tc.key) })).toContainText('Verifies ' + req.key)
  await page.getByLabel('Name of the run').fill('Regression UL')
  await page.getByRole('button', { name: 'Create run' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Regression UL' })).toBeVisible()
})

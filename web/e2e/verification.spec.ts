// Journey: verification correlations. A requirement with a test criterion
// and an inspection criterion is verified only when both have passing
// evidence: the test through its link, the inspection through a
// verification record recorded in the requirement's Verification tab.
// System elements show the verification per element.
import { expect, items, test } from './helpers'

test('a requirement is verified when every criterion has evidence, and elements count it', async ({ page, api, project }) => {
  await api.put(`/projects/${project.id}/verification-settings`, { strict_criteria: true })
  const req = await api.post(`/projects/${project.id}/requirements`, {
    title: 'Sealed housing',
    description: 'The housing MUST be sealed.',
    status: 'implemented',
    create_test: false,
    criteria: [
      { description: 'Leak test passes', method: 'test' },
      { description: 'Seal drawing checked', method: 'inspection' },
    ],
  })
  const tc = await api.post(`/projects/${project.id}/tests`, { title: 'Leak test', status: 'ready', steps: [{ action: 'Pressurise', expected: 'No leak' }] })
  const link = await api.post(`/projects/${project.id}/links`, { requirement_id: req.id, test_case_id: tc.id, criterion_id: req.criteria[0].id })
  expect(link.criteria).toHaveLength(1)
  const run = await api.post(`/projects/${project.id}/runs`, { name: 'R1', test_ids: [tc.id] })
  const [ex] = items(await api.get(`/runs/${run.id}/executions`))
  await api.patch(`/executions/${ex.id}`, { status: 'passed' })

  await page.goto(`/p/${project.id}/requirements?item=${req.id}`)
  const drawer = page.locator('.p-drawer')
  await drawer.getByRole('tab', { name: 'Verification' }).click()
  const panel = drawer.locator('.verification-panel')
  await expect(panel).toContainText('1 of 2 criteria verified')
  await expect(panel).toContainText('Criterion 2 has no evidence.')

  // record the inspection of criterion 2
  await panel.getByRole('button', { name: 'Record evidence for criterion 2' }).click()
  const dialog = page.getByRole('dialog', { name: 'Record evidence' })
  await expect(dialog).toBeVisible()
  await dialog.getByLabel('Performed by').fill('Maria Berg')
  await dialog.getByLabel('Note').fill('Drawing D-12 rev B checked')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()
  await expect(panel).toContainText('2 of 2 criteria verified')
  await expect(panel).toContainText('By Maria Berg')
  // the status followed the evidence
  await expect.poll(async () => (await api.get(`/requirements/${req.id}`)).status).toBe('verified')

  // a system element with the requirement allocated
  await page.goto(`/p/${project.id}/verification?tab=elements`)
  await page.getByRole('button', { name: 'Add element' }).click()
  const edialog = page.getByRole('dialog', { name: 'Add element' })
  await edialog.getByLabel('Name').fill('Power unit')
  await edialog.getByRole('button', { name: 'Save' }).click()
  await expect(edialog).toBeHidden()
  const els = items(await api.get(`/projects/${project.id}/elements`))
  expect(els.map((e) => e.name)).toEqual(['Power unit'])
  await api.put(`/requirements/${req.id}/elements`, { element_ids: [els[0].id] })
  await page.reload()
  await expect(page.locator('.elem-row')).toContainText('1/1 verified')

  // the hierarchy lists the requirement as verified
  await page.goto(`/p/${project.id}/verification`)
  await expect(page.locator('.tree-row').filter({ hasText: req.key })).toContainText('Verified')
})

// Journey: change impact. Changing a requirement that another one refines
// asks for confirmation with everything that becomes suspect or stale, and
// the Impact tab of the refining requirement shows its suspect relation.
// A critical defect on a test blocks the requirement it verifies.
import { expect, items, settled, test } from './helpers'

test('confirms the impact of a change and shows suspect relations, stale results and blocking defects', async ({ page, api, project }) => {
  const pp = `/projects/${project.id}`
  const a = await api.post(`${pp}/requirements`, { title: 'Sign in', description: 'The user MUST sign in.', status: 'implemented' })
  const b = await api.post(`${pp}/requirements`, { title: 'Sign in with a passkey', description: 'The user SHOULD sign in with a passkey.', status: 'implemented' })
  await api.post(`/requirements/${b.id}/relations`, { target_id: a.id, type: 'refines' })
  const tc = await api.post(`${pp}/tests`, { title: 'Sign in works', status: 'ready' })
  await api.post(`${pp}/links`, { requirement_id: a.id, test_case_id: tc.id })
  const run = await api.post(`${pp}/runs`, { name: 'Smoke', test_ids: [tc.id] })
  const [ex] = items(await api.get(`/runs/${run.id}/executions`))
  await api.patch(`/executions/${ex.id}`, { status: 'passed' })

  // change the description of A: the dialog lists B, the test and its result
  await page.goto(`/p/${project.id}/requirements?item=${a.id}`)
  await settled(page)
  const editor = page.getByRole('dialog').first()
  await editor.getByLabel('Description', { exact: true }).fill('The user MUST sign in with two factors.')
  await editor.getByRole('button', { name: 'Save', exact: true }).click()
  const confirm = page.getByRole('dialog', { name: `Save the change of ${a.key}?` })
  await expect(confirm).toBeVisible()
  await expect(confirm).toContainText('Requirements that become suspect')
  await expect(confirm).toContainText(b.key)
  await expect(confirm).toContainText(tc.key)
  await expect(confirm).toContainText('Results that go stale')
  await confirm.getByRole('button', { name: 'Save and mark' }).click()
  await expect(page.getByText('Requirement saved')).toBeVisible()
  await expect(confirm).toBeHidden()

  // A: its result is stale (a badge next to the key)
  await expect(editor.getByRole('button', { name: '1 stale result' })).toBeVisible()

  // B: the Impact tab shows the suspect relation; confirming clears it
  await page.goto(`/p/${project.id}/requirements?item=${b.id}`)
  await settled(page)
  const editorB = page.getByRole('dialog').first()
  await editorB.getByRole('tab', { name: /Impact/ }).click()
  const panel = editorB.locator('.impact-panel')
  await expect(panel).toContainText('1 suspect relation')
  await expect(panel).toContainText('this requirement is suspect')
  await panel.getByRole('button', { name: `Confirm the relation with ${a.key}` }).click()
  await expect(page.getByText('1 relation confirmed')).toBeVisible()
  await expect(panel).not.toContainText('this requirement is suspect')

  // a critical defect on the test blocks A
  await api.post(`${pp}/defects`, { title: 'Second factor is skipped', severity: 'critical', links: [{ entity_type: 'test', entity_id: tc.id }] })
  await page.goto(`/p/${project.id}/requirements?item=${a.id}`)
  await settled(page)
  const editorA = page.getByRole('dialog').first()
  await expect(editorA.getByRole('button', { name: 'Blocked by 1 defect' })).toBeVisible()
  await editorA.getByRole('button', { name: 'Blocked by 1 defect' }).click()
  await expect(editorA.locator('.impact-panel')).toContainText('Second factor is skipped')
  await expect(editorA.locator('.impact-panel')).toContainText('blocks')
})

test('edits the correlation settings of a project', async ({ page, api, project }) => {
  await page.goto(`/p/${project.id}/settings?tab=correlations`)
  await settled(page)
  const section = page.locator('section[aria-labelledby="h-corr"]')
  await expect(section.getByRole('heading', { name: 'Change impact, staleness and defects' })).toBeVisible()
  await section.getByLabel('Results go stale after').uncheck()
  await section.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Settings saved')).toBeVisible()
  const set = await api.get(`/projects/${project.id}/correlation-settings`)
  expect(set.stale_after_days).toBe(0)
})

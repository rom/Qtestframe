// Waivers and the Statement of Applicability: a MUST requirement failed in
// a release run; a waiver for the release is requested in the UI, approved
// with an electronic signature by a manager, and the release becomes a go
// with the requirement marked as waived. Then a decision in the SoA table.
import { expect, items, test, uniqueKey } from './helpers'

test('a waiver approved with an e-signature makes the release a go, marked as waived', async ({ page, browser, api, project, baseURL }) => {
  const base = `/projects/${project.id}`
  const req = await api.post(`${base}/requirements`, { title: 'Encrypt backups', description: 'The system MUST encrypt backups.', status: 'approved', source: 'ISO/IEC 27001:2022 A.8.13' })
  const tc = await api.post(`${base}/tests`, { title: 'Backup is encrypted', status: 'ready' })
  await api.post(`${base}/links`, { requirement_id: req.id, test_case_id: tc.id })
  const rel = await api.post(`${base}/releases`, { name: 'Spring', exit_criteria: [{ type: 'mandatory_passed' }, { type: 'no_failed' }] })
  await api.post(`/releases/${rel.id}/requirements`, { add: [req.id] })
  const run = await api.post(`${base}/runs`, { name: 'Release run', release_id: rel.id, test_ids: [tc.id] })
  for (const ex of items(await api.get(`/runs/${run.id}/executions`))) await api.patch(`/executions/${ex.id}`, { status: 'failed' })
  expect((await api.get(`/releases/${rel.id}/status`)).go).toBe(false)

  // a manager who approves waivers
  const email = `${uniqueKey('MGR').toLowerCase()}@example.com`
  const password = 'E2e-approver-1234'
  await api.post('/members', { email, role: 'manager', display_name: 'Approving Manager', password })

  // request the waiver (the requirement and release are filled in from the link)
  await page.goto(`/p/${project.id}/waivers?new=1&requirement=${req.id}&release=${rel.id}`)
  const form = page.getByRole('dialog', { name: 'New waiver' })
  await expect(form).toBeVisible()
  await form.getByLabel('Title').fill('Spring without encrypted backups')
  await expect(form.getByText(req.key)).toBeVisible()
  await form.getByLabel('Justification').fill('The key management service arrives in the summer release.')
  await form.getByLabel('Compensating measures').fill('Backups are stored offline in a locked safe.')
  await form.getByLabel('Approvers').fill(email)
  await form.getByLabel('Request approval now').check()
  await form.getByRole('button', { name: 'Create' }).click()
  const detail = page.getByRole('dialog', { name: /Spring without encrypted backups/ })
  await expect(detail).toBeVisible()
  await expect(detail.getByText('Requested', { exact: true })).toBeVisible()
  // the requester cannot decide it
  await expect(detail.getByRole('button', { name: 'Approve or reject' })).toHaveCount(0)
  const waiver = items(await api.get(`${base}/waivers`))[0]

  // the manager signs the approval
  const ctx = await browser.newContext({ baseURL, storageState: { cookies: [], origins: [] } })
  const mgr = await ctx.newPage()
  const login = await mgr.request.post('/api/v1/auth/login', { data: { email, password } })
  expect(login.ok()).toBeTruthy()
  const me = await (await mgr.request.get('/api/v1/auth/me')).json()
  await mgr.request.patch('/api/v1/auth/me', { data: { preferences: { intro_done: true } }, headers: { 'X-CSRF-Token': me.csrf_token } })
  await mgr.goto(`/p/${project.id}/waivers?waiver=${waiver.id}`)
  const mdetail = mgr.getByRole('dialog', { name: /Spring without encrypted backups/ })
  await mdetail.getByRole('button', { name: 'Approve or reject' }).click()
  const sign = mgr.getByRole('dialog', { name: `Decide waiver ${waiver.key}` })
  await sign.getByLabel('Your password').fill(password)
  await sign.getByRole('button', { name: 'Sign' }).click()
  await expect(sign).toBeHidden()
  await expect(mdetail.getByText('Approved', { exact: true })).toBeVisible()
  await expect(mdetail.getByText('In force')).toBeVisible()
  await ctx.close()

  // the release: go, the requirement accepted as waived
  await expect.poll(async () => (await api.get(`/releases/${rel.id}/status`)).go).toBe(true)
  await page.goto(`/p/${project.id}/releases`)
  await page.getByRole('button', { name: 'Spring' }).first().click()
  const status = page.locator('#release-detail')
  await expect(status.getByText('Go', { exact: true }).first()).toBeVisible()
  await expect(status.getByText(`waived (${waiver.key})`).first()).toBeVisible()
  await expect(status.locator('.rel-waivers')).toContainText(waiver.key)

  // the requirement shows the waiver
  await page.goto(`/p/${project.id}/requirements?item=${req.id}`)
  await expect(page.locator('.requirement-waivers')).toContainText(`Accepted by waiver ${waiver.key}`)
})

test('the Statement of Applicability lists the 93 Annex A controls and saves a decision typed in the grid', async ({ page, api, project }) => {
  await api.post(`/projects/${project.id}/requirements`, { title: 'Secure sign-in', status: 'approved', source: 'ISO/IEC 27001:2022 A.5.1' })
  await page.goto(`/p/${project.id}/soa`)
  // the first statement loads the catalogue on the server
  await expect(page.getByRole('heading', { name: 'ISO/IEC 27001:2022 Annex A' })).toBeVisible({ timeout: 20_000 })
  await expect(page.getByText('93 of 93 controls shown')).toBeVisible()
  const grid = page.getByRole('grid', { name: 'Statement of Applicability: ISO/IEC 27001:2022 Annex A' })
  await expect(grid.getByText('Policies for information security')).toBeVisible()
  // Owner of the first control (A.5.1): move there, type and press Enter
  await grid.focus()
  await page.keyboard.press('Control+Home')
  for (let i = 0; i < 4; i++) await page.keyboard.press('ArrowRight')
  const active = async () => page.locator(`[id="${await grid.getAttribute('aria-activedescendant')}"]`)
  await expect(await active()).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.type('CISO')
  await page.keyboard.press('Enter')
  await expect(page.getByRole('status').filter({ hasText: '1 control saved' })).toBeAttached()
  await expect
    .poll(async () => (await api.get(`/projects/${project.id}/soa`)).rows.find((r: { id: string }) => r.id === 'A.5.1')?.owner)
    .toBe('CISO')
  const row = (await api.get(`/projects/${project.id}/soa`)).rows.find((r: { id: string }) => r.id === 'A.5.1')
  expect(row.status).toBe('planned')
  expect(row.requirements).toHaveLength(1)
})

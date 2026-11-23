// Change requests: with change control enforced, saving an approved
// requirement is refused and the change is proposed in a change request
// instead; the change request is submitted and assessed, a manager on the
// change control board approves it with an electronic signature, and
// applying it changes the requirement.
import { expect, test, uniqueKey } from './helpers'

test('an approved requirement changes through a change request decided by the board', async ({ page, browser, api, project, baseURL }) => {
  const base = `/projects/${project.id}`
  const req = await api.post(`${base}/requirements`, { title: 'Lock accounts after 5 attempts', description: 'The system MUST lock an account after 5 failed attempts.', status: 'approved' })
  const email = `${uniqueKey('CCB').toLowerCase()}@example.com`
  const password = 'E2e-board-member-1234'
  await api.post('/members', { email, role: 'manager', display_name: 'Board Member', password })
  await api.put(`${base}/change-control`, { mode: 'enforce', board: { users: [email], quorum: 'all' }, signature_meaning: 'Approved by the change control board' })

  // edit in the editor: the save is refused, the change is proposed instead
  await page.goto(`/p/${project.id}/requirements?item=${req.id}`)
  const drawer = page.locator('.p-drawer')
  await expect(drawer.locator('.cc-bar')).toContainText('Under change control')
  await drawer.getByLabel('Title', { exact: true }).fill('Lock accounts after 3 attempts')
  await drawer.getByRole('button', { name: 'Save' }).click()
  const propose = page.getByRole('dialog', { name: 'Propose a change' })
  await expect(propose).toBeVisible()
  await expect(propose.locator('ins')).toContainText('3')
  await propose.getByLabel('Title').fill('Stricter lockout')
  await propose.getByLabel('Reason (justification)').fill('Security policy 4.2 requires a lockout after 3 failed attempts.')
  await propose.getByRole('button', { name: 'Propose' }).click()
  await expect(propose).toBeHidden()
  expect((await api.get(`/requirements/${req.id}`)).title).toBe('Lock accounts after 5 attempts')

  // submit and assess in the change request
  const cr = (await api.get(`${base}/change-requests`)).items[0]
  await page.goto(`/p/${project.id}/change-requests?cr=${cr.id}`)
  const detail = page.getByRole('dialog', { name: new RegExp(`${cr.key} Stricter lockout`) })
  await expect(detail).toBeVisible()
  await detail.getByRole('button', { name: 'Submit' }).click()
  await expect(detail.getByText('Submitted', { exact: true }).first()).toBeVisible()
  await detail.getByRole('button', { name: 'Assess impact' }).click()
  const assess = page.getByRole('dialog', { name: `Assess ${cr.key}` })
  await assess.getByLabel('Effort estimate').fill('1 day')
  await assess.getByRole('button', { name: 'Assess impact' }).click()
  await expect(assess).toBeHidden()
  await expect(detail.getByText('In review', { exact: true }).first()).toBeVisible()
  // the requester cannot decide
  await expect(detail.getByRole('button', { name: 'Decide…' })).toHaveCount(0)

  // the board member signs the approval
  const ctx = await browser.newContext({ baseURL, storageState: { cookies: [], origins: [] } })
  const mgr = await ctx.newPage()
  const login = await mgr.request.post('/api/v1/auth/login', { data: { email, password } })
  expect(login.ok()).toBeTruthy()
  const me = await (await mgr.request.get('/api/v1/auth/me')).json()
  await mgr.request.patch('/api/v1/auth/me', { data: { preferences: { intro_done: true } }, headers: { 'X-CSRF-Token': me.csrf_token } })
  await mgr.goto(`/p/${project.id}/change-requests?cr=${cr.id}`)
  const mdetail = mgr.getByRole('dialog', { name: new RegExp(`${cr.key} Stricter lockout`) })
  await mdetail.getByRole('button', { name: 'Decide…' }).click()
  const sign = mgr.getByRole('dialog', { name: `Decide ${cr.key}` })
  await expect(sign).toContainText('Approved by the change control board')
  await sign.getByLabel('Your password').fill(password)
  await sign.getByRole('button', { name: 'Sign' }).click()
  await expect(sign).toBeHidden()
  await expect(mdetail.getByText('Approved', { exact: true }).first()).toBeVisible()
  await ctx.close()

  // apply: the requirement changes, with a new version
  await page.reload()
  await expect(detail).toBeVisible()
  await detail.getByRole('button', { name: 'Apply' }).click()
  const apply = page.getByRole('dialog', { name: `Apply ${cr.key}` })
  await apply.getByRole('button', { name: 'Apply' }).click()
  await expect(apply).toBeHidden()
  await expect(detail.getByText('Implemented', { exact: true }).first()).toBeVisible()
  const after = await api.get(`/requirements/${req.id}`)
  expect(after.title).toBe('Lock accounts after 3 attempts')
  expect(after.version).toBeGreaterThan(req.version)
})

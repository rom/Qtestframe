// Live updates in two browsers: a change made by one user appears in the
// other user's list without reloading, the editor shows who else is on the
// item, and a newer version saved meanwhile is offered with a notice.
import { expect, test, uniqueKey, type Api } from './helpers'
import type { Browser, Page } from '@playwright/test'

const password = 'E2e-live-user-1234'

async function member(api: Api, name: string): Promise<string> {
  const email = `${uniqueKey(name.slice(0, 3).toUpperCase()).toLowerCase()}@example.com`
  await api.post('/members', { email, role: 'manager', display_name: name, password })
  return email
}

/** A browser of its own, signed in as email; returns the page and its CSRF token. */
async function signIn(browser: Browser, baseURL: string, email: string): Promise<{ page: Page; csrf: string }> {
  const ctx = await browser.newContext({ baseURL, storageState: { cookies: [], origins: [] } })
  const page = await ctx.newPage()
  const login = await page.request.post('/api/v1/auth/login', { data: { email, password } })
  expect(login.ok()).toBeTruthy()
  const me = await (await page.request.get('/api/v1/auth/me')).json()
  await page.request.patch('/api/v1/auth/me', { data: { preferences: { intro_done: true, language: 'en' } }, headers: { 'X-CSRF-Token': me.csrf_token } })
  return { page, csrf: me.csrf_token }
}

test('a change by one user appears for another without reloading', async ({ browser, api, project, baseURL }) => {
  const req = await api.post(`/projects/${project.id}/requirements`, { title: 'Original title' })
  const alice = await member(api, 'Alice Live')
  const bob = await member(api, 'Bob Live')
  const a = await signIn(browser, baseURL!, alice)
  const b = await signIn(browser, baseURL!, bob)
  const bobSaves = async (title: string) => {
    const res = await b.page.request.patch(`/api/v1/requirements/${req.id}`, { data: { title }, headers: { 'X-CSRF-Token': b.csrf } })
    expect(res.ok()).toBeTruthy()
  }

  // Alice looks at the list; Bob changes the requirement elsewhere
  // the stream opens a moment after the page has loaded
  const stream = a.page.waitForResponse((r) => r.url().includes('/api/v1/events/stream') && r.status() === 200, { timeout: 15_000 })
  await a.page.goto(`/p/${project.id}/requirements`)
  await expect(a.page.getByRole('button', { name: /Original title/ })).toBeVisible()
  await stream
  await bobSaves('Changed by Bob')
  await expect(a.page.getByRole('button', { name: /Changed by Bob/ })).toBeVisible({ timeout: 15_000 })
  await expect(a.page.getByRole('button', { name: /Original title/ })).toHaveCount(0)

  // both open the requirement: each sees the other
  await b.page.goto(`/p/${project.id}/requirements?item=${req.id}`)
  const bEditor = b.page.locator('.editor-drawer')
  await expect(bEditor.getByLabel('Title', { exact: true })).toHaveValue('Changed by Bob')
  await a.page.getByRole('button', { name: /Changed by Bob/ }).click()
  const aEditor = a.page.locator('.editor-drawer')
  await expect(aEditor.getByLabel('Title', { exact: true })).toHaveValue('Changed by Bob')
  await expect(aEditor.getByTestId('live-presence')).toContainText('Bob Live is also viewing this', { timeout: 15_000 })
  await expect(bEditor.getByTestId('live-presence')).toContainText('Alice Live is also viewing this', { timeout: 15_000 })

  // Alice starts typing: Bob sees her editing
  await aEditor.getByLabel('Title', { exact: true }).fill('Alice is typing')
  await expect(bEditor.getByTestId('live-presence')).toContainText('Alice Live is also editing this', { timeout: 15_000 })

  // Bob saves a newer version: Alice gets a notice, not a blocked editor
  await bobSaves('Saved by Bob again')
  const banner = aEditor.getByTestId('live-banner')
  await expect(banner).toContainText('Bob Live changed this item (version 3).', { timeout: 15_000 })
  await expect(banner).toContainText('Your changes are not saved yet.')
  // compare shows their field next to hers
  await banner.getByRole('button', { name: 'Compare' }).click()
  const conflict = a.page.getByRole('dialog', { name: 'Someone else changed this' })
  await expect(conflict).toContainText('Saved by Bob again')
  await expect(conflict).toContainText('Alice is typing')
  await conflict.getByRole('button', { name: 'Discard mine, load theirs' }).click()
  await expect(aEditor.getByLabel('Title', { exact: true })).toHaveValue('Saved by Bob again')
  await expect(banner).toHaveCount(0)

  await a.page.context().close()
  await b.page.context().close()
})

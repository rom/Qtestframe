// Journey 6: switching the language from the user menu and choosing a
// colour theme. Preferences are stored per user, so this test signs in as a
// user of its own and does not change the administrator's UI for the others.
import { expect, openUserMenu, test, uniqueKey } from './helpers'

test.use({ storageState: { cookies: [], origins: [] } })

test('switches to Swedish and to another theme, and keeps both after a reload', async ({ page, api, project }) => {
  const email = `${uniqueKey('PREF').toLowerCase()}@example.com`
  const password = 'E2e-preferences-1234'
  await api.post('/members', { email, role: 'tester', display_name: 'Preference Tester', password })
  const login = await page.request.post('/api/v1/auth/login', { data: { email, password } })
  expect(login.ok()).toBeTruthy()
  const me = await (await page.request.get('/api/v1/auth/me')).json()
  await page.request.patch('/api/v1/auth/me', { data: { preferences: { intro_done: true } }, headers: { 'X-CSRF-Token': me.csrf_token } })

  await page.goto(`/p/${project.id}/dashboard`)
  const nav = page.getByRole('navigation', { name: 'Main navigation' })
  await expect(nav.getByRole('link', { name: 'Requirements' })).toBeVisible()

  // language: user menu → Svenska
  await openUserMenu(page)
  await page.getByRole('menuitem', { name: 'Svenska' }).click()
  const sv = page.getByRole('navigation', { name: 'Huvudnavigering' })
  await expect(sv.getByRole('link', { name: 'Översikt' })).toBeVisible()
  await expect(sv.getByRole('link', { name: 'Krav' })).toBeVisible()
  await expect(sv.getByRole('link', { name: 'Testfall' })).toBeVisible()
  await expect(sv.getByRole('link', { name: 'Testkörningar' })).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('lang', 'sv')

  // theme: the picker in the top bar
  await page.getByRole('combobox', { name: 'Färgtema' }).click()
  await page.getByRole('option', { name: 'Marinblått' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'navy')
  await expect(page.locator('html')).toHaveClass(/app-dark/)

  // both are stored as preferences of the user
  await expect
    .poll(async () => (await (await page.request.get('/api/v1/auth/me')).json()).user.preferences)
    .toMatchObject({ language: 'sv', theme: 'navy' })
  await page.reload()
  await expect(page.getByRole('navigation', { name: 'Huvudnavigering' }).getByRole('link', { name: 'Krav' })).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'navy')
})

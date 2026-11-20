// Journey 1: signing in and out.
import { ADMIN, expect, openUserMenu, test } from './helpers'

// these tests start signed out
test.use({ storageState: { cookies: [], origins: [] } })

test('signs in, sees the projects and signs out again', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login/)
  await page.locator('input[type=email]').fill(ADMIN.email)
  await page.locator('input[type=password]').fill(ADMIN.password)
  await page.locator('input[type=password]').press('Enter')

  await expect(page).not.toHaveURL(/\/login/)
  // the demo project from seed-demo is listed
  await expect(page.getByRole('main').getByText('DEMO', { exact: true }).first()).toBeVisible()

  await openUserMenu(page)
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await expect(page).toHaveURL(/\/login/)
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()

  // the session is gone: protected pages lead back to the login
  await page.goto('/projects')
  await expect(page).toHaveURL(/\/login\?redirect=/)
})

test('shows an error for a wrong password and stays on the login page', async ({ page }) => {
  await page.goto('/login')
  await page.locator('input[type=email]').fill(ADMIN.email)
  await page.locator('input[type=password]').fill('wrong-password-1234')
  await page.locator('input[type=password]').press('Enter')

  const error = page.locator('#login-error')
  await expect(error).toBeVisible()
  await expect(error).toContainText(/invalid credentials/i)
  await expect(page).toHaveURL(/\/login/)
  await expect(page.locator('input[type=email]')).toHaveAttribute('aria-invalid', 'true')
})

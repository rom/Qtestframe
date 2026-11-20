// Journey 4: planning a run, executing it in the runner and seeing the
// result on the run, the dashboard and the traceability page.
import { expect, settled, test } from './helpers'

test('creates a run, executes it with the keyboard and reports the pass', async ({ page, api, project }) => {
  // a ready test verifying an approved requirement may be executed with the default settings
  const req = await api.post(`/projects/${project.id}/requirements`, {
    title: 'Sign in with e-mail',
    description: 'The system MUST let users sign in with e-mail and password.',
    status: 'approved',
    create_test: false,
  })
  const tc = await api.post(`/projects/${project.id}/tests`, {
    title: 'Sign in with valid credentials',
    execution_type: 'manual',
    status: 'ready',
    steps: [
      { action: 'Open the login page', expected: 'The login form is shown' },
      { action: 'Enter a valid e-mail and password and press Enter', expected: 'The dashboard is shown' },
    ],
  })
  await api.post(`/projects/${project.id}/links`, { requirement_id: req.id, test_case_id: tc.id })

  // plan the run in the UI
  await page.goto(`/p/${project.id}/runs`)
  await settled(page)
  await page.getByRole('button', { name: 'New run' }).click()
  const dialog = page.getByRole('dialog', { name: 'New test run' })
  await dialog.getByLabel('Name', { exact: true }).fill('Release 1.0 smoke')
  await dialog.getByRole('button', { name: 'Pick tests…' }).click()
  const picker = page.getByRole('dialog', { name: 'Select tests' })
  await expect(picker.getByRole('row', { name: new RegExp(tc.key) })).toBeVisible()
  await picker.getByRole('row', { name: new RegExp(tc.key) }).getByRole('checkbox').check()
  await picker.getByRole('button', { name: 'Add', exact: true }).click()
  await expect(dialog.getByText(tc.key)).toBeVisible()
  await dialog.getByRole('button', { name: 'Create run' }).click()

  // the run page
  await expect(page.getByRole('heading', { level: 1, name: 'Release 1.0 smoke' })).toBeVisible()
  await expect(page.getByRole('row', { name: new RegExp(tc.key) })).toContainText('Not run')
  await page.getByRole('button', { name: 'Execute', exact: true }).click()

  // the runner: mark both steps passed with the keyboard shortcut P
  await expect(page).toHaveURL(/\/runs\/[^/]+\/exec\/[^/]+$/)
  await expect(page.getByRole('heading', { level: 1 })).toContainText(tc.key)
  await expect(page.getByRole('heading', { name: 'Step 1 of 2' })).toBeVisible()
  const pass = page.getByRole('button', { name: 'Pass (P)' })
  await expect(pass).toBeEnabled()
  await page.getByRole('heading', { name: 'Steps' }).click()
  await page.keyboard.press('p')
  await expect(page.getByRole('heading', { name: 'Step 2 of 2' })).toBeVisible()
  await page.keyboard.press('p')
  await expect(pass).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByText('2/2', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Save & finish' }).click()

  // back on the run: completed, the execution passed
  await expect(page.getByRole('heading', { level: 1, name: 'Release 1.0 smoke' })).toBeVisible()
  await expect(page.getByRole('combobox', { name: 'Run status' })).toContainText('Completed')
  await expect(page.getByRole('row', { name: new RegExp(tc.key) })).toContainText('Passed')

  // the dashboard counts the passed execution and the covered requirement
  await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: 'Dashboard' }).click()
  await settled(page)
  await expect(page.getByRole('img', { name: /^Execution results\..*Passed: 1/ })).toBeAttached()
  await expect(page.getByRole('img', { name: /^Requirement coverage\..*Passed: 1/ })).toBeAttached()

  // traceability: the requirement is covered by a passed test
  await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: 'Traceability' }).click()
  await settled(page)
  const row = page.getByRole('table', { name: /Requirements in rows/ }).getByRole('row', { name: new RegExp(`\\b${req.key}\\b`) })
  await expect(row.getByRole('cell', { name: 'Passed', exact: true })).toBeVisible()
  await expect(row.getByRole('button', { name: `Link ${req.key} to ${tc.key}` })).toHaveAttribute('aria-pressed', 'true')
})

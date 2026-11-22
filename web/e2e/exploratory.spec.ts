// Exploratory testing: plan a session, run it with the keyboard (timer and
// typed notes), raise a defect from a note, stop and debrief it.
import { expect, settled, test } from './helpers'

test('plans, runs and debriefs an exploratory session with the keyboard', async ({ page, api, project }) => {
  const req = await api.post(`/projects/${project.id}/requirements`, { title: 'Pay by card', status: 'approved', create_test: false })

  await page.goto(`/p/${project.id}/exploratory`)
  await settled(page)
  await page.getByRole('button', { name: 'New session' }).click()
  const dialog = page.getByRole('dialog', { name: 'New session' })
  await dialog.getByLabel('Charter').fill('Explore checkout with expired cards')
  await dialog.getByLabel('Mission').fill('Explore payment with expired cards to discover unclear errors')
  await dialog.getByRole('button', { name: 'Add requirements' }).click()
  const picker = page.getByRole('dialog', { name: 'Add requirements' })
  await picker.getByRole('row', { name: new RegExp(req.key) }).getByRole('checkbox').check()
  await picker.getByRole('button', { name: 'Add', exact: true }).click()
  await dialog.getByRole('button', { name: 'Plan session' }).click()

  // the session runner
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Explore checkout with expired cards')
  await page.getByRole('heading', { name: 'Session time' }).click()
  await page.keyboard.press('t') // start
  await expect(page.getByRole('button', { name: 'Pause' })).toBeVisible()
  await page.getByRole('heading', { name: 'Session time' }).click()
  await page.keyboard.press('q') // a question note
  await expect(page.getByRole('radio', { name: /Question/ })).toHaveAttribute('aria-checked', 'true')
  await page.keyboard.type('Should the expiry date be shown?')
  await page.keyboard.press('Control+Enter')
  await expect(page.getByRole('heading', { name: 'Notes (1)' })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.keyboard.press('b') // a bug note
  await page.keyboard.type('Expired card shows a raw error code')
  await page.keyboard.press('Control+Enter')
  await expect(page.getByRole('heading', { name: 'Notes (2)' })).toBeVisible()

  // a defect from the bug note, linked to the session and the requirement
  await page.getByRole('button', { name: /^Raise defect from the note/ }).last().click()
  const defect = page.getByRole('dialog', { name: 'Raise defect' })
  await expect(defect.getByLabel('Title')).toHaveValue('Expired card shows a raw error code')
  await defect.getByRole('button', { name: 'Raise defect' }).click()
  await expect(page.getByRole('link', { name: /-BUG-\d+/ }).first()).toBeVisible()

  // stop and debrief
  await page.getByRole('button', { name: 'Stop' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Stop' }).click()
  const debrief = page.getByRole('dialog', { name: /^Debrief/ })
  await debrief.getByLabel('Summary').fill('Expired cards give raw error codes.')
  await debrief.getByRole('button', { name: 'Save debrief' }).click()
  await expect(page.getByText('Debriefed', { exact: true }).first()).toBeVisible()

  const sessions = await api.get(`/projects/${project.id}/exploratory-sessions`)
  const x = (sessions as { items: { status: string; note_count: number; bug_count: number; debrief: { outcome: string } }[] }).items[0]
  expect(x.status).toBe('debriefed')
  expect(x.note_count).toBe(2)
  expect(x.bug_count).toBe(1)
  expect(x.debrief.outcome).toBe('failed')
})

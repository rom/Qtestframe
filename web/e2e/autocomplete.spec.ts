// Autocomplete: a value saved in a text field is suggested the next time
// the field is filled in, and the profile preference turns it off.
import { AxeBuilder } from '@axe-core/playwright'
import { expect, settled, test, uniqueKey } from './helpers'

test('suggests a requirement title saved before, and not when turned off', async ({ page, project, api }) => {
  const word = uniqueKey('AC')
  const saved = `${word} the export finishes within five seconds`
  await page.goto(`/p/${project.id}/requirements`)
  await settled(page)

  // save a requirement: its title goes into the history
  await page.getByRole('button', { name: 'New requirement' }).click()
  const editor = page.getByRole('dialog')
  await editor.getByLabel('Title', { exact: true }).fill(saved)
  await editor.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByText('Requirement created')).toBeVisible()
  await expect.poll(async () => (await api.get(`/suggest?field=requirement.title&q=${word}&project_id=${project.id}`)).total).toBe(1)
  await editor.getByRole('button', { name: 'Close', exact: true }).last().click()
  await expect(editor).toBeHidden()

  // a new one: typing the beginning suggests the saved title
  await page.getByRole('button', { name: 'New requirement' }).click()
  const title = editor.getByLabel('Title', { exact: true })
  await expect(title).toHaveAttribute('role', 'combobox')
  await title.pressSequentially(word.toLowerCase().slice(0, 6), { delay: 20 })
  const list = editor.getByRole('listbox', { name: 'Suggestions' })
  await expect(list.getByRole('option', { name: saved })).toBeVisible()
  await expect(title).toHaveAttribute('aria-expanded', 'true')
  // nothing chosen until an arrow key: then Enter takes it
  await title.press('ArrowDown')
  await expect(title).toHaveAttribute('aria-activedescendant', /suggest-list/)
  // the open combobox and its listbox pass axe (WCAG 2.1 A/AA)
  const axe = await new AxeBuilder({ page }).include('.suggest-input').withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(axe.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
  await title.press('Enter')
  await expect(title).toHaveValue(saved)
  await expect(list).toBeHidden()
  // in an empty field ArrowDown lists recent values; Escape closes the list, not the editor
  await title.fill('') // (also leaves the new form unchanged)
  await title.press('ArrowDown')
  await expect(list.getByRole('option', { name: saved })).toBeVisible()
  await title.press('Escape')
  await expect(list).toBeHidden()
  await expect(editor).toBeVisible()

  // turned off in the profile: no list and no requests
  await api.patch('/auth/me', { preferences: { autocomplete: 'off' } })
  try {
    await page.reload()
    await settled(page)
    const asked: string[] = []
    page.on('request', (r) => {
      if (r.url().includes('/api/v1/suggest')) asked.push(r.url())
    })
    await page.getByRole('button', { name: 'New requirement' }).click()
    const plain = editor.getByLabel('Title', { exact: true })
    await expect(plain).not.toHaveAttribute('role', 'combobox')
    await plain.pressSequentially(word.toLowerCase().slice(0, 6), { delay: 20 })
    await page.waitForTimeout(600)
    await expect(editor.getByRole('listbox', { name: 'Suggestions' })).toHaveCount(0)
    expect(asked).toEqual([])
  } finally {
    await api.patch('/auth/me', { preferences: { autocomplete: null } })
  }
})

test('the profile offers the preference and the clear button', async ({ page }) => {
  await page.goto('/profile')
  await settled(page)
  await expect(page.getByLabel('Autocomplete suggestions')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Clear my autocomplete history' })).toBeVisible()
})

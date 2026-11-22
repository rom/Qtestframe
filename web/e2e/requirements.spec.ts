// Journey 2: writing a requirement with the language analysis, editing it
// and undoing the edit with Ctrl+Z.
import { expect, settled, test } from './helpers'

test('creates, finds, edits and undoes the edit of a requirement', async ({ page, project }) => {
  await page.goto(`/p/${project.id}/requirements`)
  await settled(page)

  await page.getByRole('button', { name: 'New requirement' }).click()
  const editor = page.getByRole('dialog')
  await expect(editor).toBeVisible()
  await editor.getByLabel('Title', { exact: true }).fill('Export to PDF')
  await editor.getByLabel('Description', { exact: true }).fill('The system MUST export every report as a PDF file.')

  // the language analysis finds the RFC 2119 keyword while typing
  const analysis = editor.getByRole('region', { name: 'Requirement language analysis' })
  await expect(analysis).toBeVisible()
  await expect(analysis.locator('.kw', { hasText: 'MUST' })).toBeVisible()
  await expect(analysis).toContainText(/Detected\s*MUST/)

  await editor.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByText('Requirement created')).toBeVisible()
  // the editor now shows the saved requirement with its key
  await expect(editor.locator('form .key').first()).toHaveText(new RegExp(`^${project.key}-`))
  const key = ((await editor.locator('form .key').first().textContent()) ?? '').trim()
  // the Close button in the form footer (the drawer's own × sits under the toasts)
  await editor.getByRole('button', { name: 'Close', exact: true }).last().click()
  await expect(editor).toBeHidden()

  // it is in the list
  const row = page.getByRole('row').filter({ hasText: key })
  await expect(row.getByRole('button', { name: new RegExp(`${key}.*Export to PDF`) })).toBeVisible()

  // edit the title
  await row.getByRole('button', { name: /Export to PDF/ }).click()
  await expect(editor).toBeVisible()
  const title = editor.getByLabel('Title', { exact: true })
  await expect(title).toHaveValue('Export to PDF')
  await title.fill('Export reports to PDF')
  await editor.getByRole('button', { name: 'Save', exact: true }).click()
  // the title is substance: the link of the linked test becomes suspect, confirmed before saving
  const impact = page.getByRole('dialog', { name: `Save the change of ${key}?` })
  await expect(impact).toContainText('Tests whose links become suspect')
  await impact.getByRole('button', { name: 'Save and mark' }).click()
  await expect(page.getByText('Requirement saved')).toBeVisible()
  await editor.getByRole('button', { name: 'Close', exact: true }).last().click()
  await expect(editor).toBeHidden()
  await expect(page.getByRole('button', { name: new RegExp(`${key}.*Export reports to PDF`) })).toBeVisible()

  // Ctrl+Z outside text fields undoes the edit
  await page.locator('h1').first().click()
  await page.keyboard.press('Control+z')
  await expect(page.getByRole('button', { name: new RegExp(`${key}.*Export to PDF`) })).toBeVisible()
  await expect(page.getByRole('button', { name: /Export reports to PDF/ })).toHaveCount(0)

  // and the server has the old title again
  await page.reload()
  await settled(page)
  await expect(page.getByRole('button', { name: new RegExp(`${key}.*Export to PDF`) })).toBeVisible()
})

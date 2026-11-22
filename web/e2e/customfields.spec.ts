// Journey: custom fields and item templates. A project admin defines a
// custom field, creates a requirement from a built-in template with custom
// values, finds it with cf.<key> and sees the field as a list column.
import { expect, settled, test } from './helpers'

test('defines a custom field and creates a requirement from a template', async ({ page, api, project }) => {
  // a number field with a unit (set up through the API)
  await api.post(`/projects/${project.id}/custom-fields`, { entity_type: 'requirement', label: 'Mass', type: 'number', unit: 'kg' })

  // a text field shown in lists, through the settings page
  await page.goto(`/p/${project.id}/settings?tab=customFields`)
  await settled(page)
  await page.getByRole('button', { name: 'New field' }).click()
  const dialog = page.getByRole('dialog', { name: 'New custom field' })
  await dialog.getByLabel('Label', { exact: true }).fill('Supplier')
  await expect(dialog.getByLabel('Key', { exact: true })).toHaveValue('supplier')
  await dialog.getByLabel('Show in lists').check()
  await dialog.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByRole('table', { name: /Custom fields of Requirements/ })).toContainText('cf.supplier')

  // New requirement: the built-in template fills in the form
  await page.goto(`/p/${project.id}/requirements`)
  await settled(page)
  await page.getByRole('button', { name: 'New requirement' }).click()
  const editor = page.getByRole('dialog')
  await editor.getByRole('combobox', { name: 'Template' }).click()
  await page.getByRole('option', { name: /Requirement with acceptance criteria/ }).click()
  await expect(editor.getByLabel('Title', { exact: true })).toHaveValue('<Subject> shall <capability>')
  await editor.getByLabel('Title', { exact: true }).fill('Bracket carries the motor')
  const fields = editor.getByRole('group', { name: 'Custom fields' })
  await fields.getByLabel('Supplier').fill('Acme Ltd')
  await fields.getByLabel(/Mass/).fill('2.5')
  await editor.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByText('Requirement created')).toBeVisible()
  await editor.getByRole('button', { name: 'Close', exact: true }).last().click()

  // the server stored typed values
  const reqs = (await api.get(`/projects/${project.id}/requirements?q=${encodeURIComponent('cf.mass>2')}`)).items
  expect(reqs).toHaveLength(1)
  expect(reqs[0].custom_fields).toEqual({ supplier: 'Acme Ltd', mass: 2.5 })
  expect(reqs[0].criteria_count).toBe(1)

  // the query language finds it and the list shows the column
  await page.goto(`/p/${project.id}/requirements?q=${encodeURIComponent('cf.supplier:"Acme"')}`)
  await settled(page)
  await expect(page.getByRole('columnheader', { name: 'Supplier' })).toBeVisible()
  const row = page.getByRole('row').filter({ hasText: 'Bracket carries the motor' })
  await expect(row).toContainText('Acme Ltd')
})

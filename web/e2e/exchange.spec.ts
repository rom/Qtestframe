// Exchanging data with other tools: import a ReqIF document in the shape
// DOORS exports, check the attribute mapping and the diff preview, save,
// then import a changed version (an update, not new requirements); export
// the Excel round-trip workbook and import it back after someone else
// changed an item (a conflict that is not saved).
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, items, settled, test } from './helpers'

const fixture = (name: string) => readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../../internal/interop/testdata', name))

test('imports and re-imports a ReqIF document with a diff preview', async ({ page, api, project }) => {
  await page.goto(`/p/${project.id}/requirements`)
  await settled(page)
  await page.getByRole('button', { name: 'Import', exact: true }).click()
  await page.getByRole('menuitem', { name: /ReqIF from DOORS or Polarion/ }).click()
  const dialog = page.getByRole('dialog', { name: 'Import ReqIF' })
  await expect(dialog).toBeVisible()
  const doc = fixture('doors.reqif')
  await dialog.locator('input[type="file"]').setInputFiles({ name: 'system-requirements.reqif', mimeType: 'application/xml', buffer: doc })

  // the attributes and their fields, the rows and the relations
  await expect(dialog.getByRole('heading', { name: 'Attributes' })).toBeVisible()
  await expect(dialog.getByText('ReqIF.ChapterName')).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'New 3' })).toBeVisible()
  await expect(dialog.getByText('2 to add, 0 to remove, 0 unchanged.')).toBeVisible()
  await expect(dialog.getByRole('checkbox', { name: 'Headings become folders (3)' })).toBeChecked()
  await dialog.getByRole('button', { name: 'Save 3 rows' }).click()
  await expect(dialog.getByRole('status')).toContainText('3 created, 0 updated')
  await dialog.locator('.p-dialog-footer').getByRole('button', { name: 'Close' }).click()

  const reqs = items<{ id: string; title: string; folder_path: string }>(await api.get(`/projects/${project.id}/requirements`))
  expect(reqs).toHaveLength(3)
  expect(reqs.map((r) => r.folder_path).sort()).toEqual(['Braking / Emergency braking', 'Braking / Emergency braking', 'General'])
  expect(items(await api.get(`/projects/${project.id}/relations`))).toHaveLength(2)

  // DOORS changed one requirement: the re-import updates it
  const changed = doc.toString('utf8').replace('The driver SHOULD be warned acoustically', 'The driver SHALL be warned acoustically and visually')
  await page.getByRole('button', { name: 'Import', exact: true }).click()
  await page.getByRole('menuitem', { name: /ReqIF from DOORS or Polarion/ }).click()
  await dialog.locator('input[type="file"]').setInputFiles({ name: 'system-requirements.reqif', mimeType: 'application/xml', buffer: Buffer.from(changed) })
  await expect(dialog.getByText('Imported before: 3 objects of this document are updated.')).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Changed 1' })).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'New 0' })).toBeVisible()
  await expect(dialog.locator('ins').first()).toContainText('acoustically and visually')
  await dialog.getByRole('button', { name: 'Save 1 row' }).click()
  await expect(dialog.getByRole('status')).toContainText('0 created, 1 updated')
  expect(items(await api.get(`/projects/${project.id}/requirements`))).toHaveLength(3)
})

test('shows a conflict when the Excel workbook is older than the item', async ({ page, api, project }) => {
  const a = await api.post<{ id: string; key: string }>(`/projects/${project.id}/requirements`, { title: 'Alpha', create_test: false })
  await api.post(`/projects/${project.id}/requirements`, { title: 'Bravo', create_test: false })
  await page.goto(`/p/${project.id}/requirements`)
  await settled(page)
  await page.getByRole('button', { name: 'Import', exact: true }).click()
  const downloaded = page.waitForEvent('download')
  await page.getByRole('menuitem', { name: 'Export for editing in Excel' }).click()
  const workbook = await downloaded
  const path = await workbook.path()
  expect(workbook.suggestedFilename()).toMatch(/requirements-roundtrip.*\.xlsx$/)

  // someone saves Alpha after the export
  await api.patch(`/requirements/${a.id}`, { title: 'Alpha (edited elsewhere)' })

  await page.getByRole('button', { name: 'Import', exact: true }).click()
  await page.getByRole('menuitem', { name: /Import an edited Excel workbook/ }).click()
  const dialog = page.getByRole('dialog', { name: 'Import edited requirements' })
  await dialog.locator('input[type="file"]').setInputFiles({ name: workbook.suggestedFilename(), mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: readFileSync(path!) })
  await expect(dialog.getByRole('button', { name: 'Conflict 1' })).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Unchanged 1' })).toBeVisible()
  await expect(dialog.getByText('Exported version 1, now 2')).toBeVisible()
  // nothing to save unless the conflict is overwritten
  await expect(dialog.getByRole('button', { name: 'Save', exact: true })).toBeDisabled()
  await dialog.getByRole('checkbox', { name: /Overwrite the conflicts/ }).check()
  await dialog.getByRole('button', { name: 'Save 1 row' }).click()
  await expect(dialog.getByRole('status')).toContainText('1 updated')
  const after = await api.get<{ title: string }>(`/requirements/${a.id}`)
  expect(after.title).toBe('Alpha')
})

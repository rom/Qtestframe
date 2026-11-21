// The spreadsheet view of the requirements list: edit a cell in place, paste
// a 2×2 block from the clipboard (tab-separated, as Excel copies it), refuse
// an invalid paste without saving anything, and undo a paste in one step.
import { expect, items, settled, test } from './helpers'

test.use({ permissions: ['clipboard-read', 'clipboard-write'] })

test('edits a cell and pastes a 2×2 block in the spreadsheet view', async ({ page, api, project }) => {
  for (const title of ['Alpha', 'Bravo', 'Charlie']) await api.post(`/projects/${project.id}/requirements`, { title, priority: 'medium', create_test: false })
  const keyed = async () => {
    const list = items<{ key: string; title: string; description: string; priority: string; version: number }>(await api.get(`/projects/${project.id}/requirements`))
    return list.sort((a, b) => a.key.localeCompare(b.key, undefined, { numeric: true }))
  }

  await page.goto(`/p/${project.id}/requirements?view=sheet`)
  await settled(page)
  const grid = page.getByRole('grid', { name: 'Requirements spreadsheet' })
  await expect(grid).toBeVisible()
  await expect(page.getByRole('button', { name: 'Spreadsheet', exact: true })).toHaveAttribute('aria-pressed', 'true')
  // key and title of every row, plus the header and the new-item row
  await expect(grid.getByRole('row')).toHaveCount(5)
  await expect(grid).toHaveAttribute('aria-rowcount', '5')

  // edit: move to the title of the first row and type over it
  await grid.focus()
  await page.keyboard.press('Control+Home')
  await page.keyboard.press('ArrowRight')
  const active = async () => page.locator(`[id="${await grid.getAttribute('aria-activedescendant')}"]`)
  await expect(await active()).toHaveText('Alpha')
  await expect(await active()).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.type('Alpha one')
  await page.keyboard.press('Enter')
  await expect(page.getByRole('status').filter({ hasText: `Saved Title of ${project.key}-REQ-1` })).toBeVisible()
  expect((await keyed())[0].title).toBe('Alpha one')
  // Enter moved down to the second row
  await expect(await active()).toHaveText('Bravo')

  // a read-only cell says so and is not changed
  await page.keyboard.press('Home')
  await expect(await active()).toHaveAttribute('aria-readonly', 'true')

  // paste a 2×2 block (title and description of rows 2 and 3)
  await page.keyboard.press('ArrowRight')
  await page.evaluate(() => navigator.clipboard.writeText('Bravo two\tFirst line\r\nCharlie three\t"Two\nlines"\r\n'))
  await page.keyboard.press('Control+v')
  await expect(page.getByRole('status').filter({ hasText: 'Saved 4 cells' })).toBeVisible()
  const after = await keyed()
  expect(after.map((r) => [r.title, r.description])).toEqual([
    ['Alpha one', ''],
    ['Bravo two', 'First line'],
    ['Charlie three', 'Two\nlines'],
  ])
  await expect(grid.locator('[role="gridcell"]', { hasText: 'First line' })).toBeVisible()

  // an invalid priority refuses the whole paste: nothing is saved
  await page.keyboard.press('Control+Home')
  for (let i = 0; i < 4; i++) await page.keyboard.press('ArrowRight')
  await expect(await active()).toHaveText('Medium')
  await page.evaluate(() => navigator.clipboard.writeText('High\nUrgent\n'))
  await page.keyboard.press('Control+v')
  await expect(page.getByRole('alert').filter({ hasText: '"Urgent" is not a valid Priority' })).toBeVisible()
  expect((await keyed()).map((r) => r.priority)).toEqual(['medium', 'medium', 'medium'])

  // the 2×2 paste is one undo step
  await grid.focus()
  await page.keyboard.press('Control+z')
  await expect.poll(async () => (await keyed()).map((r) => r.title)).toEqual(['Alpha one', 'Bravo', 'Charlie'])
  expect((await keyed()).map((r) => r.description)).toEqual(['', '', ''])

  // the layout is remembered: the list comes back as a spreadsheet
  await page.goto(`/p/${project.id}/requirements`)
  await settled(page)
  await expect(page.getByRole('grid', { name: 'Requirements spreadsheet' })).toBeVisible()
  await page.getByRole('button', { name: 'List', exact: true }).click()
  await expect(page.getByRole('grid', { name: 'Requirements spreadsheet' })).toHaveCount(0)
})

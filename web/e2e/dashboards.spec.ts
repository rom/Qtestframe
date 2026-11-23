// Customisable dashboards: a personal dashboard gets a widget added, moved
// with the keyboard and removed; the layout is saved on the server. The
// built-in overview stays as it was for everyone else.
import { expect, items, settled, test } from './helpers'

test('add, move and remove a widget on a personal dashboard', async ({ page, api, project }) => {
  await api.post(`/projects/${project.id}/requirements`, { title: 'Login', status: 'draft' })
  await page.goto(`/p/${project.id}/dashboard`)
  await settled(page)
  // the built-in layouts of the former dashboard
  await expect(page.getByRole('tab', { name: /Overview/ })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Key figures' })).toBeVisible()

  // a new, empty personal dashboard opens in edit mode
  await page.getByRole('button', { name: 'More dashboard actions' }).click()
  await page.getByRole('menuitem', { name: 'New dashboard' }).click()
  const dlg = page.getByRole('dialog', { name: 'New dashboard' })
  await dlg.getByLabel('Name').fill('Mine')
  await dlg.getByRole('button', { name: 'Create' }).click()
  await expect(page.getByRole('tab', { name: /Mine/ })).toHaveAttribute('aria-selected', 'true')

  // add two widgets
  for (const name of ['Saved query', 'Note']) {
    await page.getByRole('button', { name: 'Add widget' }).click()
    const add = page.getByRole('dialog', { name: 'Add widget' })
    await add.getByLabel('Filter widgets').fill(name)
    await add.getByRole('button', { name: new RegExp(`^${name}`) }).click()
  }
  await expect(page.getByRole('heading', { name: 'Saved query' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Note' })).toBeVisible()

  // move the note with the keyboard: it was placed right of the query
  const handle = page.getByRole('button', { name: 'Move Note', exact: true })
  await handle.focus()
  await page.keyboard.press('Home')
  await expect(page.locator('[aria-live="assertive"]')).toContainText('Note: column 1, row 1')
  await page.keyboard.press('ArrowDown')
  await expect(page.locator('[aria-live="assertive"]')).toContainText('Note: column 1, row 2')
  // resize it with Shift+arrows
  await page.keyboard.press('Shift+ArrowRight')
  await expect(page.locator('[aria-live="assertive"]')).toContainText('Note: 7 columns')

  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Dashboard saved').first()).toBeVisible()
  let mine = items(await api.get(`/projects/${project.id}/dashboards`)).find((d: { name: string }) => d.name === 'Mine')
  const note = mine.widgets.find((w: { type: string }) => w.type === 'note')
  expect(note).toMatchObject({ x: 0, y: 1, w: 7 })
  // the query widget was pushed below the note
  expect(mine.widgets.find((w: { type: string }) => w.type === 'saved_query').y).toBeGreaterThanOrEqual(4)
  // the saved query shows the project's requirement
  await expect(page.getByRole('link', { name: /Login/ })).toBeVisible()

  // remove the query widget
  await page.getByRole('button', { name: 'Customise' }).click()
  await page.getByRole('button', { name: 'Remove Saved query' }).click()
  await expect(page.getByRole('heading', { name: 'Saved query' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Dashboard saved').first()).toBeVisible()
  mine = items(await api.get(`/projects/${project.id}/dashboards`)).find((d: { name: string }) => d.name === 'Mine')
  expect(mine.widgets.map((w: { type: string }) => w.type)).toEqual(['note'])

  // the overview is unchanged
  const overview = items(await api.get(`/projects/${project.id}/dashboards`))[0]
  expect(overview).toMatchObject({ id: 'overview', stored: false })
})

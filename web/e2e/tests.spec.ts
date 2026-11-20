// Journey 3: designing a manual test, copying it, deleting the copy and
// bringing it back with Ctrl+Z.
import { expect, settled, test } from './helpers'

test('creates a manual test with steps, copies it and undoes deleting the copy', async ({ page, project }) => {
  await page.goto(`/p/${project.id}/tests`)
  await settled(page)

  await page.getByRole('button', { name: 'New test' }).click()
  const editor = page.getByRole('dialog')
  await expect(editor).toBeVisible()
  await editor.getByLabel('Title', { exact: true }).fill('Pay with a credit card')
  await expect(editor.getByRole('button', { name: 'Manual' })).toHaveAttribute('aria-pressed', 'true')
  for (let n = 0; n < 2; n++) await editor.getByRole('button', { name: 'Add step' }).click()
  await editor.getByLabel('Action of step 1').fill('Put an item in the basket and open the checkout')
  await editor.getByLabel('Expected result of step 1').fill('The checkout shows the item and the total')
  await editor.getByLabel('Action of step 2').fill('Pay with the test card 4111 1111 1111 1111')
  await editor.getByLabel('Expected result of step 2').fill('The order is confirmed')
  await editor.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByText('Test created')).toBeVisible()

  // the editor now shows the saved test
  const drawer = page.locator('.editor-drawer')
  const keyOf = async () => ((await drawer.locator('form .key').first().textContent()) ?? '').trim()
  await expect(drawer.locator('form .key').first()).toHaveText(new RegExp(`^${project.key}-`))
  const original = await keyOf()
  await expect(drawer.getByLabel('Action of step 2')).toHaveValue(/test card/)

  // copy: the editor switches to the copy, a new draft with the same steps
  await drawer.getByRole('button', { name: 'Copy', exact: true }).click()
  await expect(page.getByText('Test copied')).toBeVisible()
  await expect(drawer.locator('form .key').first()).not.toHaveText(original)
  const copy = await keyOf()
  await expect(drawer.getByLabel('Action of step 1')).toHaveValue(/basket/)
  await expect(drawer.getByLabel('Action of step 2')).toHaveValue(/test card/)

  const listed = (key: string) => page.getByRole('button', { name: new RegExp(`^Open ${key}\\b`) })
  await expect(listed(original)).toBeVisible()
  await expect(listed(copy)).toBeVisible()

  // delete the copy (with confirmation)
  await drawer.getByRole('button', { name: 'Delete', exact: true }).click()
  const confirm = page.getByRole('alertdialog', { name: 'Please confirm' })
  await expect(confirm).toContainText(`Delete test ${copy}?`)
  await confirm.getByRole('button', { name: 'Delete' }).click()
  await expect(page.getByText('Test deleted')).toBeVisible()
  await expect(drawer).toBeHidden()
  await expect(listed(copy)).toHaveCount(0)
  await expect(listed(original)).toBeVisible()

  // Ctrl+Z restores it from the trash, with the same key
  await page.locator('h1').first().click()
  await page.keyboard.press('Control+z')
  await expect(listed(copy)).toBeVisible()
  await page.reload()
  await settled(page)
  await expect(listed(copy)).toBeVisible()
  await expect(listed(original)).toBeVisible()
})

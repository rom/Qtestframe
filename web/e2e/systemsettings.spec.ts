// System settings (super administrators): find a setting with the search,
// change it, confirm the change in the dialog that lists it, and see it
// applied (the token policy reaches /meta and the API token dialog at
// once). The value is put back at the end, as the settings are shared by
// all tests.
import { expect, test } from './helpers'

test('finds, changes and applies a system setting after a confirmation', async ({ page, api }) => {
  const before = (await api.get('/admin/settings')).settings.find((s: { key: string }) => s.key === 'auth.token_max_days')
  try {
    await page.goto('/admin/system')
    await page.getByRole('searchbox', { name: 'Search settings' }).fill('token lifetime')
    const hit = page.getByRole('button', { name: /Longest token lifetime/ })
    await expect(hit).toBeVisible()
    await hit.click()
    await expect(page.getByRole('heading', { name: 'API tokens', level: 2 })).toBeVisible()

    const field = page.locator('.sys-field[data-key="auth.token_max_days"]')
    await field.getByRole('spinbutton').fill('120')
    await field.getByRole('spinbutton').press('Tab')
    await page.getByRole('button', { name: 'Save section' }).click()

    const dialog = page.getByRole('dialog', { name: 'Save these changes?' })
    await expect(dialog.getByRole('rowheader', { name: /Longest token lifetime/ })).toBeVisible()
    await expect(dialog.getByRole('cell', { name: '120' })).toBeVisible()
    await dialog.getByRole('button', { name: 'Save changes' }).click()
    await expect(dialog).toBeHidden()
    await expect(field.getByText('Changed here')).toBeVisible()

    // applied at once: the server's metadata and the new-token dialog
    await expect.poll(async () => (await api.get('/meta')).token_max_days).toBe(120)
    await page.goto('/admin/tokens')
    await page.getByRole('button', { name: 'New token' }).click()
    await expect(page.getByRole('dialog').getByText('at most 120 days')).toBeVisible()

    // and audited with the old and the new value
    const audit = await api.get('/audit?limit=100')
    expect(JSON.stringify(audit)).toContain('auth.token_max_days')
  } finally {
    await api.put('/admin/settings', { values: { 'auth.token_max_days': before?.stored ? before.value : null } })
  }
})

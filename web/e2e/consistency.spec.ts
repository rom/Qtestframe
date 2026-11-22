// Built-in consistency checks and quality metrics: the analytics page shows
// a suspected duplicate with its fix actions, "Mark as duplicate" resolves
// it, the checks can be added to the project rules, and the quality
// metrics show their key figures with a table view of a chart.
import { expect, items, test } from './helpers'

test('a suspected duplicate is marked as duplicate from the analytics', async ({ page, api, project }) => {
  const older = await api.post(`/projects/${project.id}/requirements`, { title: 'Lockout', description: 'The system MUST lock an account after five failed sign-in attempts.' })
  const newer = await api.post(`/projects/${project.id}/requirements`, { title: 'Lockout', description: 'The system MUST lock an account after five failed sign-in attempt.' })
  await api.post(`/projects/${project.id}/defects`, { title: 'Lockout bypassed', found_in: 'production', links: [{ entity_type: 'requirement', entity_id: older.id }] })

  await page.goto(`/p/${project.id}/analytics`)
  const section = page.locator('section.consistency')
  await expect(section.getByRole('heading', { name: 'Consistency checks' })).toBeVisible()
  await expect(section).toContainText('9 built-in checks are not part of the project rules yet')
  const dup = section.locator('li.check', { hasText: 'Suspected duplicate requirements' })
  await expect(dup).toContainText('1 violation')
  await dup.getByRole('button', { name: /Show 1 violation/ }).click()
  await expect(dup.getByRole('link', { name: `${newer.key} Lockout` }).first()).toBeVisible()
  await expect(dup.getByRole('link', { name: `${older.key} Lockout` })).toBeVisible()
  await dup.getByRole('button', { name: 'Mark as duplicate' }).click()
  await expect(dup).toContainText('0 violations')
  const rels = items(await api.get(`/requirements/${newer.id}/relations`))
  expect(rels.some((r: { type: string; target_id: string }) => r.type === 'duplicates' && r.target_id === older.id)).toBe(true)

  // add the checks to the project's rules
  await section.getByRole('button', { name: 'Add built-in checks' }).click()
  await expect(section).not.toContainText('not part of the project rules')
  const rules = items(await api.get(`/projects/${project.id}/trace-rules`))
  expect(rules.filter((r: { kind: string }) => r.kind === 'duplicate_suspect')).toHaveLength(1)

  // quality metrics: key figures and a chart as a table
  const quality = page.locator('section.quality')
  await expect(quality.getByRole('heading', { name: 'Quality metrics' })).toBeVisible()
  const escaped = quality.locator('.qkpi', { hasText: 'Escaped defects' })
  await expect(escaped).toContainText('100 %')
  await expect(escaped).toContainText('1 of 1 defects')
  const card = quality.locator('section.card', { has: page.getByRole('heading', { name: 'Escaped defects' }) })
  await card.getByRole('button', { name: 'Show as table' }).click()
  await expect(card.locator('table.data-table')).toBeVisible()
})

test('built-in checks show on the traceability rules with their fixes', async ({ page, api, project }) => {
  const parent = await api.post(`/projects/${project.id}/requirements`, { title: 'Old parent', description: 'The system MAY show a banner.', status: 'obsolete' })
  const child = await api.post(`/projects/${project.id}/requirements`, { title: 'Child', description: 'The system SHOULD show a banner on start.' })
  await api.post(`/requirements/${child.id}/relations`, { target_id: parent.id, type: 'refines' })
  await api.post(`/projects/${project.id}/trace-rules/builtins`, { kinds: ['obsolete_parent'] })

  await page.goto(`/p/${project.id}/traceability?view=rules`)
  const rule = page.locator('.trace-rules .rule', { hasText: 'Active requirement with an obsolete parent' })
  await expect(rule).toContainText('Built-in')
  await rule.getByRole('button', { name: /Show 1 violation/ }).click()
  await expect(rule.getByRole('link', { name: `${parent.key} Old parent` })).toBeVisible()
  await rule.getByRole('button', { name: 'Set obsolete' }).click()
  await expect(rule).not.toContainText('Set obsolete')
  expect((await api.get(`/requirements/${child.id}`)).status).toBe('obsolete')
})

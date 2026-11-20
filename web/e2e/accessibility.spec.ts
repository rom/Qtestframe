// Journey 7: accessibility smoke test. axe-core (WCAG 2.1 A/AA) on the
// dashboard and the requirements of the seeded demo project; serious and
// critical violations fail. The full matrix of themes, languages and
// viewports is scripts/a11y-check.mjs (`npm run a11y`).
import { AxeBuilder } from '@axe-core/playwright'
import type { Page } from '@playwright/test'
import { expect, items, settled, test, type Project } from './helpers'

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']

async function seriousViolations(page: Page) {
  const result = await new AxeBuilder({ page }).withTags(TAGS).analyze()
  return result.violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => `[${v.impact}] ${v.id}: ${v.help}\n    ${v.nodes.slice(0, 5).map((n) => n.target.join(' ')).join('\n    ')}`)
}

let demo: Project

test.beforeEach(async ({ api }) => {
  demo ??= items<Project>(await api.get('/projects')).find((p) => p.key === 'DEMO')!
  expect(demo, 'the seed-demo project DEMO exists').toBeTruthy()
})

for (const [name, path, ready] of [
  ['dashboard', 'dashboard', 'canvas'],
  ['requirements', 'requirements', 'button.item-title'],
] as const) {
  test(`${name} has no serious or critical axe violations`, async ({ page }) => {
    await page.goto(`/p/${demo.id}/${path}`)
    await settled(page)
    await expect(page.locator(ready).first()).toBeVisible()
    expect(await seriousViolations(page)).toEqual([])
  })
}

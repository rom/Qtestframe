// BDD authoring: a Gherkin test written in the editor with inline errors
// and step completion from the step library, then run with one iteration
// per scenario and Examples row.
import { AxeBuilder } from '@axe-core/playwright'
import { expect, items, settled, test } from './helpers'

test('writes a Gherkin test with completion and inline errors and runs its scenarios', async ({ page, api, project }) => {
  await api.post(`/projects/${project.id}/step-definitions`, { pattern: 'the customer pays {int} EUR', keyword: 'when', description: 'Pays at the checkout' })

  await page.goto(`/p/${project.id}/tests`)
  await settled(page)
  await page.getByRole('button', { name: 'New test' }).click()
  const editor = page.getByRole('dialog')
  await editor.getByLabel('Title', { exact: true }).fill('Checkout')
  await editor.getByRole('button', { name: 'Gherkin', exact: true }).click()
  await expect(editor.getByRole('button', { name: 'Gherkin', exact: true })).toHaveAttribute('aria-pressed', 'true')

  const input = editor.getByRole('textbox', { name: 'Gherkin' })
  await expect(input).toHaveValue(/Feature: Checkout/)

  // an error is shown with its line
  await input.fill('Feature: Checkout\n  Scenario: Pay\n    Given a customer\n    something odd\n')
  await expect(editor.getByRole('button', { name: 'Go to line 4' })).toBeVisible()
  await expect(input).toHaveAttribute('aria-invalid', 'true')

  // completion from the step library
  await input.fill('Feature: Checkout\n  Scenario Outline: Pay <amount>\n    Given a customer\n    When the cu')
  await input.press('End')
  await input.press('Control+Space')
  const option = editor.getByRole('option', { name: /the customer pays \{int\} EUR/ })
  await expect(option).toBeVisible()
  await input.press('Enter')
  await expect(input).toHaveValue(/When the customer pays \{int\} EUR$/)
  // the parameter is selected: typing replaces it
  await input.pressSequentially('<amount>')
  await input.press('End')
  await input.press('Enter')
  await input.pressSequentially('Examples:\n|amount|\n|5|\n|10|')
  await input.press('Alt+Shift+F')
  await expect(input).toHaveValue(/\| amount \|\n\s+\| 5      \|/)
  await expect(editor.getByRole('status').filter({ hasText: '2 scenarios to run' })).toBeVisible()
  // "a customer" is not in the library: offered to add
  await expect(editor.getByRole('button', { name: 'Add a customer to the step library' })).toBeVisible()

  // no serious accessibility problems in the editor (highlighting, errors, suggestions)
  const axe = await new AxeBuilder({ page }).include('.gherkin-editor').withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(axe.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => v.id)).toEqual([])

  await editor.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByText('Test created')).toBeVisible()

  // a run gives each Examples row an iteration with the steps
  const tests = items(await api.get(`/projects/${project.id}/tests`))
  const t = tests.find((x) => x.title === 'Checkout')
  expect(t.format).toBe('gherkin')
  const run = await api.post(`/projects/${project.id}/runs`, { name: 'BDD run', test_ids: [t.id] })
  const ex = items(await api.get(`/runs/${run.id}/executions`))[0]
  const exec = await api.get(`/executions/${ex.id}`)
  expect(exec.iterations.map((i: { name: string }) => i.name)).toEqual(['Pay 5', 'Pay 10'])
  expect(exec.step_results[1].action).toBe('When the customer pays 5 EUR')
})

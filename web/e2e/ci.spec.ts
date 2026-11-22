// CI/CD: the CI setup panel with the templates of the project, the
// server-side posting to GitLab (token write-only) and the quality gate a
// pipeline checks.
import { expect, settled, test } from './helpers'

test('shows the CI setup, saves the GitLab posting and evaluates the gate', async ({ page, api, project }) => {
  const pp = `/projects/${project.id}`
  const req = await api.post<{ id: string; key: string }>(`${pp}/requirements`, { title: 'Checkout', obligation: 'must', create_test: false })
  const tc = await api.post<{ id: string; key: string }>(`${pp}/tests`, { title: 'Checkout works', execution_type: 'automated', status: 'ready' })
  await api.post(`${pp}/links`, { requirement_id: req.id, test_case_id: tc.id })
  const run = await api.post<{ id: string }>(`${pp}/runs`, { name: 'Pipeline 1', test_ids: [tc.id] })
  await api.post(`/runs/${run.id}/results?build_id=1&commit=abcdef1234`, { results: [{ test_key: tc.key, status: 'failed' }] })

  await page.goto(`/p/${project.id}/settings?tab=project`)
  await settled(page)
  const setup = page.locator('section.ci-setup')
  await expect(setup.getByRole('heading', { name: 'CI setup' })).toBeVisible()
  await expect(setup.getByRole('row', { name: /QTF_PROJECT/ })).toContainText(project.key)
  await expect(setup.locator('pre.ci-template').first()).toContainText(`QTF_PROJECT: "${project.key}"`)
  await setup.getByRole('button', { name: 'GitHub Actions' }).click()
  await expect(setup.locator('pre.ci-template').first()).toContainText('QTF_GITHUB_TOKEN')

  // results back to GitLab: the token is required and never shown again
  const posting = page.locator('section.ci-posting')
  await posting.getByLabel('Post commit statuses and merge request notes to GitLab').check()
  await posting.getByLabel('GitLab project').fill('group/app')
  await posting.getByRole('button', { name: 'Save' }).click()
  await expect(posting).toContainText('Enter the access token.')
  await posting.getByLabel('Access token').fill('example-token-value')
  await posting.getByLabel('Gate conditions').fill('must, no_failed')
  await posting.getByRole('button', { name: 'Save' }).click()
  await expect(posting.getByRole('status')).toContainText('token saved')
  await expect(posting.getByLabel('Access token')).toHaveValue('')
  const cs = await api.get<{ ci: { project: string; conditions: string[] }; has_ci_token: boolean; ci_token?: string }>(`${pp}/code-settings`)
  expect(cs.ci.project).toBe('group/app')
  expect(cs.ci.conditions).toEqual(['must', 'no_failed'])
  expect(cs.has_ci_token).toBe(true)
  expect(cs.ci_token).toBeUndefined()

  // the gate of the pipeline's run fails on the MUST requirement
  const gate = await api.post<{ pass: boolean; conditions: { type: string; met: boolean }[] }>(`${pp}/gate`, { run: 'Pipeline 1' })
  expect(gate.pass).toBe(false)
  expect(gate.conditions.find((c) => c.type === 'mandatory_passed')?.met).toBe(false)
})

import { describe, expect, it } from 'vitest'
import type { FileInfo } from './types'
import {
  EMPTY_COUNTS,
  evidenceCount,
  isCompleteSpec,
  jobActive,
  jobProgress,
  pollDelay,
  showTraceCommand,
  skippedByCode,
  skippedText,
  validBaseUrl,
  validateWebTests,
  variableEnvName,
  WEB_STARTER,
  webEvidence,
  webTestsDirty,
  webTestsForm,
  webTestsPayload,
} from './webTests'

const file = (name: string, content_type: string, tags: string[] | null): FileInfo => ({
  id: name,
  project_id: 'p',
  kind: 'attachment',
  name,
  content_type,
  size: 1,
  sha256: '',
  description: '',
  tags,
  uploaded_by: '',
  created_at: '',
  url: '',
  links: null,
})

describe('web tests', () => {
  it('tells a complete spec from the body of a test', () => {
    expect(isCompleteSpec("test('a', async ({ page }) => {})")).toBe(true)
    expect(isCompleteSpec("test.describe('x', () => {})")).toBe(true)
    expect(isCompleteSpec("await test.step('s', async () => {})")).toBe(false)
    expect(isCompleteSpec('await page.goto("/")')).toBe(false)
    expect(isCompleteSpec(WEB_STARTER)).toBe(false)
  })

  it('summarises jobs', () => {
    expect(jobActive({ state: 'queued' })).toBe(true)
    expect(jobActive({ state: 'running' })).toBe(true)
    expect(jobActive({ state: 'passed' })).toBe(false)
    expect(jobActive(null)).toBe(false)
    expect(jobProgress(null)).toBe(0)
    expect(jobProgress(EMPTY_COUNTS)).toBe(0)
    expect(jobProgress({ ...EMPTY_COUNTS, done: 1, total: 3 })).toBe(33)
    expect(pollDelay(0)).toBe(1000)
    expect(pollDelay(-1)).toBe(1000)
    expect(pollDelay(3)).toBe(2500)
    expect(pollDelay(100)).toBe(5000)
    const skipped = [
      { execution_id: '1', test_key: 'A-1', reason: 'no', code: 'not_ready' },
      { execution_id: '2', test_key: 'A-2', configuration_name: 'Firefox', reason: 'busy', code: 'active' },
      { execution_id: '3', test_key: 'A-3', reason: 'no', code: 'not_ready' },
    ]
    expect(skippedByCode(skipped)).toEqual({ not_ready: 2, active: 1 })
    expect(skippedByCode(null)).toEqual({})
    expect(skippedText(skipped, 2)).toBe('A-1: no\nA-2 (Firefox): busy\n… +1')
    expect(skippedText(undefined)).toBe('')
  })

  it('finds the evidence of a web test', () => {
    const files = [
      file('screenshot.png', 'image/png', ['web-test', 'screenshot']),
      file('trace.zip', 'application/zip', ['web-test', 'trace']),
      file('video.webm', 'video/webm', ['web-test', 'video']),
      file('other.png', 'image/png', ['web-test']),
      file('clip.webm', 'video/webm', ['web-test']),
      file('t2.zip', 'application/octet-stream', ['web-test']),
      file('notes.txt', 'text/plain', ['web-test']),
      file('mine.png', 'image/png', null),
    ]
    const e = webEvidence(files)
    expect(e.screenshots.map((f) => f.name)).toEqual(['screenshot.png', 'other.png'])
    expect(e.traces.map((f) => f.name)).toEqual(['trace.zip', 't2.zip'])
    expect(e.videos.map((f) => f.name)).toEqual(['video.webm', 'clip.webm'])
    expect(evidenceCount(e)).toBe(6)
    expect(evidenceCount(webEvidence(null))).toBe(0)
    expect(showTraceCommand()).toBe('npx playwright show-trace trace.zip')
  })

  it('edits the settings without exposing secrets', () => {
    const saved = {
      base_url: 'https://a.example.com',
      environment_urls: [{ environment: 'prod', base_url: 'https://b.example.com' }],
      variables: [
        { name: 'USER', value: 'alice', secret: false, has_value: true },
        { name: 'PASS', value: '', secret: true, has_value: true },
      ],
    }
    const f = webTestsForm(saved)
    expect(f.variables[1]).toEqual({ name: 'PASS', value: '', secret: true, has_value: true })
    expect(webTestsDirty(f, saved)).toBe(false)
    f.environment_urls.push({ environment: ' ', base_url: '' })
    f.variables.push({ name: '', value: '', secret: false })
    expect(webTestsDirty(f, saved)).toBe(false)
    f.variables[0].value = 'bob'
    expect(webTestsDirty(f, saved)).toBe(true)
    expect(webTestsPayload(f)).toEqual({
      base_url: 'https://a.example.com',
      environment_urls: [{ environment: 'prod', base_url: 'https://b.example.com' }],
      variables: [
        { name: 'USER', value: 'bob', secret: false },
        { name: 'PASS', value: '', secret: true },
      ],
    })
    expect(webTestsForm(null)).toEqual({ base_url: '', environment_urls: [], variables: [] })
    expect(webTestsForm({ base_url: 'x', environment_urls: null, variables: null }).variables).toEqual([])
  })

  it('validates the settings', () => {
    expect(validBaseUrl('https://x.example.com/app')).toBe(true)
    expect(validBaseUrl('http://localhost:8080')).toBe(true)
    expect(validBaseUrl('ftp://x')).toBe(false)
    expect(validBaseUrl('https://')).toBe(false)
    expect(validBaseUrl('https://a b')).toBe(false)
    expect(validBaseUrl('http://[::1')).toBe(false)
    expect(
      validateWebTests({
        base_url: 'nope',
        environment_urls: [
          { environment: 'Prod', base_url: 'https://p.example.com' },
          { environment: 'prod', base_url: 'x' },
          { environment: '', base_url: 'https://q.example.com' },
          { environment: '', base_url: '' },
        ],
        variables: [
          { name: 'A', value: '1', secret: false },
          { name: 'a', value: '2', secret: false },
          { name: '9x', value: '', secret: false },
          { name: 'LONG', value: 'x'.repeat(5000), secret: true },
          { name: '', value: '', secret: false },
        ],
      }),
    ).toEqual({
      base_url: 'url',
      'env.1.environment': 'duplicate',
      'env.1.base_url': 'url',
      'env.2.environment': 'environment',
      'var.1.name': 'duplicate',
      'var.2.name': 'name',
      'var.3.value': 'too_long',
    })
    expect(validateWebTests({ base_url: '', environment_urls: [], variables: [] })).toEqual({})
    expect(variableEnvName(' api-key ')).toBe('QTF_VAR_API_KEY')
  })
})

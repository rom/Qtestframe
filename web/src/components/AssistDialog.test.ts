import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import { i18n } from '@/i18n'
import AssistDialog, { type AssistApply } from './AssistDialog.vue'
import type { AiFeature } from '@/lib/ai'

globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as never
window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
const meta = { feature: 'x', prompt_version: 'v', provider: 'anthropic', model: 'fake-model', usage: { input_tokens: 120, output_tokens: 40 }, retried: false, duration_ms: 1500, cost: 0.0006 }
const fetchMock = vi.fn()
const posted: { url: string; body: Record<string, unknown> }[] = []

async function settle() {
  for (let i = 0; i < 10; i++) {
    await Promise.resolve()
    await nextTick()
  }
  await new Promise((r) => setTimeout(r, 30))
  await nextTick()
}

beforeEach(() => {
  posted.length = 0
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    posted.push({ url, body })
    if (url.endsWith('/ai/draft-tests'))
      return json({
        meta,
        result: {
          requirement_id: 'r1',
          requirement_key: 'DEMO-REQ-1',
          tests: [{ title: 'Valid sign-in', description: 'Happy path', preconditions: 'An account', test_data: 'alice', expected_result: 'Dashboard', steps: [{ action: 'Sign in', data: '', expected: 'Dashboard' }], criteria: [1], criterion_ids: ['c1'] }],
        },
      })
    if (url.endsWith('/ai/improve-requirement'))
      return json({
        meta,
        result: {
          original: { title: 'Login', description: 'Login should be fast.' },
          title: 'Login',
          description: 'The system MUST show the dashboard within 2 s.',
          explanation: 'Measurable.',
          before: { language: 'en', obligation: 'should', score: 60, issues: [{ rule: 'ambiguous-term', severity: 'warning', message: 'ambiguous term "fast"', offset: 16, length: 4, params: { term: 'fast' } }] },
          after: { language: 'en', obligation: 'must', score: 100, issues: [] },
          fixed: ['ambiguous-term'],
          new: [],
          claimed: ['ambiguous-term'],
        },
      })
    if (url.endsWith('/ai/suggest-mappings')) return json({ error: 'at most 30 AI requests per hour', code: 'rate_limited' }, 429)
    if (url.endsWith('/tests')) return json({ id: 't9' }, 201)
    if (url.endsWith('/links')) return json({ id: 'l9' }, 201)
    return new Response('{}', { status: 404 })
  })
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

async function mount(feature: AiFeature, mode: 'create' | 'emit' = 'create') {
  const visible = ref(false)
  const applied: AssistApply[] = []
  const created: number[] = []
  setActivePinia(createPinia())
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    render: () =>
      h(AssistDialog, {
        projectId: 'p1',
        feature,
        body: { requirement_id: 'r1' },
        mode,
        visible: visible.value,
        'onUpdate:visible': (v: boolean) => (visible.value = v),
        onApply: (a: AssistApply) => applied.push(a),
        onCreated: (n: number) => created.push(n),
      }),
  })
  app.use(PrimeVue, { theme: 'none' }).use(ToastService).use(ConfirmationService).use(i18n)
  app.mount(el)
  visible.value = true
  await settle()
  return { visible, applied, created }
}

const button = (text: string) => [...document.querySelectorAll('.p-dialog button')].find((b) => b.textContent?.includes(text)) as HTMLButtonElement | undefined

describe('AssistDialog', () => {
  it('drafts tests and creates the accepted ones linked to the requirement', async () => {
    const { visible, created } = await mount('draft_tests')
    expect(posted[0].url).toBe('/api/v1/projects/p1/ai/draft-tests')
    expect(posted[0].body).toMatchObject({ requirement_id: 'r1', lang: 'en' })
    const dlg = document.querySelector('.p-dialog') as HTMLElement
    expect((dlg.querySelector('input[aria-label="Title of test 1"]') as HTMLInputElement).value).toBe('Valid sign-in')
    expect(dlg.textContent).toContain('fake-model · 120 input and 40 output tokens')
    expect(dlg.textContent).toContain('estimated cost 0.0006')
    // every form control has a name
    for (const c of dlg.querySelectorAll('input, textarea')) {
      const id = c.getAttribute('id')
      const named = c.getAttribute('aria-label') || (id && dlg.querySelector(`label[for="${id}"]`))
      expect(named, c.outerHTML).toBeTruthy()
    }
    button('Create 1 draft test')!.click()
    await settle()
    const test = posted.find((p) => p.url.endsWith('/projects/p1/tests'))!
    expect(test.body).toMatchObject({ title: 'Valid sign-in', status: 'draft', preconditions: 'An account\n\nTest data: alice' })
    expect(posted.find((p) => p.url.endsWith('/links'))!.body).toMatchObject({ requirement_id: 'r1', test_case_id: 't9', criteria: [{ criterion_id: 'c1' }] })
    expect(created).toEqual([1])
    expect(visible.value).toBe(false)
  })

  it('hands one draft to the test editor', async () => {
    const { applied } = await mount('draft_tests', 'emit')
    button('Use this draft')!.click()
    await settle()
    expect(applied[0]).toMatchObject({ feature: 'draft_tests', test: { title: 'Valid sign-in' } })
    expect(posted.some((p) => p.url.endsWith('/tests'))).toBe(false)
  })

  it('shows the diff and the findings fixed of an improved requirement', async () => {
    const { applied } = await mount('improve_requirement')
    const dlg = document.querySelector('.p-dialog') as HTMLElement
    expect(dlg.querySelector('.assist-diff ins')?.textContent).toBeTruthy()
    expect(dlg.querySelector('.assist-diff del')?.textContent).toBeTruthy()
    expect(dlg.textContent).toContain('Findings fixed (1)')
    expect(dlg.textContent).toContain('Quality score 60 → 100')
    button('Use this text')!.click()
    await settle()
    expect(applied[0]).toEqual({ feature: 'improve_requirement', title: 'Login', description: 'The system MUST show the dashboard within 2 s.' })
  })

  it('explains errors', async () => {
    await mount('suggest_mappings')
    const dlg = document.querySelector('.p-dialog') as HTMLElement
    expect(dlg.querySelector('.p-message')?.textContent).toContain('You have used all AI requests of this hour')
    expect(button('Ask again')).toBeTruthy()
  })
})

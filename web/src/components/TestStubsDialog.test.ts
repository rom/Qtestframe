import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import { i18n } from '@/i18n'
import TestStubsDialog from './TestStubsDialog.vue'
import { shellQuote, stubsCommand, type StubResponse } from '@/lib/stubs'

vi.mock('@/lib/download', async (orig) => ({ ...(await orig<typeof import('@/lib/download')>()), saveBlob: vi.fn() }))

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const answer = (files: StubResponse['files'], marked = 0): StubResponse => ({
  framework: { id: 'pytest', name: 'pytest', language: 'python', runner: 'pytest', script_language: 'python', parametrised: true },
  files,
  marked,
})
const file = (key: string, path: string) => ({ key, path, content: `# qtestframe:begin ${key} pytest body=1\ndef test_${key.replace(/-/g, '_')}():\n`, framework: 'pytest', test_id: key.toLowerCase() })

const fetchMock = vi.fn()
const bodies = () => fetchMock.mock.calls.map((c) => JSON.parse(String((c[1] as RequestInit).body ?? '{}')))

async function settle() {
  for (let i = 0; i < 10; i++) {
    await Promise.resolve()
    await nextTick()
  }
}

beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (_url: string, init: RequestInit) => {
    const b = JSON.parse(String(init.body))
    if (b.format === 'zip') return new Response(new Blob(['zip']), { status: 200, headers: { 'Content-Type': 'application/zip', 'Content-Disposition': 'attachment; filename="st-pytest-stubs.zip"' } })
    const files = [file('ST-TC-1', `${b.dir ? b.dir + '/' : ''}test_ST_TC_1.py`), file('ST-TC-2', 'test_ST_TC_2.py')]
    return new Response(JSON.stringify(answer(files, b.mark_script ? 2 : 0)), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
  vi.stubGlobal('fetch', fetchMock)
  URL.createObjectURL ??= (() => 'blob:x') as never
  URL.revokeObjectURL ??= (() => {}) as never
})
afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

async function mount(props: { ids?: string[]; query?: string; canWrite?: boolean } = {}) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }, { path: '/help/:page', name: 'help', component: { render: () => null } }] })
  const visible = ref(true)
  const marked = vi.fn()
  const app = createApp({
    render: () =>
      h(TestStubsDialog, {
        projectId: 'p1',
        ids: props.ids ?? [],
        query: props.query ?? 'execution_type:automated',
        shown: 2,
        canWrite: props.canWrite ?? true,
        visible: visible.value,
        'onUpdate:visible': (v: boolean) => (visible.value = v),
        onMarked: marked,
      }),
  })
  app.use(pinia).use(router).use(PrimeVue, { theme: 'none' }).use(ToastService).use(ConfirmationService).use(i18n)
  app.mount(el)
  await settle()
  return { app, marked }
}

describe('TestStubsDialog', () => {
  it('previews the stubs of the listed tests and downloads a zip', async () => {
    const { app } = await mount()
    expect(fetchMock.mock.calls[0][0]).toContain('/api/v1/projects/p1/tests/stubs')
    expect(bodies()[0]).toEqual({ framework: expect.any(String), format: 'json', dir: 'tests', query: 'execution_type:automated' })
    expect(document.querySelector('[data-testid="stubs-scope"]')?.textContent).toContain('the 2 tests of the list')
    const pre = document.querySelector('[data-testid="stubs-code"]') as HTMLElement
    expect(pre.textContent).toContain('qtestframe:begin ST-TC-1')
    expect(pre.getAttribute('aria-label')).toBe('Content of tests/test_ST_TC_1.py')
    expect(pre.getAttribute('tabindex')).toBe('0')
    expect(document.body.textContent).toContain('2 files')
    expect(document.querySelector('label[for="stubs-framework"]')).not.toBeNull()
    expect(document.querySelector('#stubs-dir')?.getAttribute('aria-describedby')).toBe('stubs-dir-hint')
    // no package for pytest
    expect(document.querySelector('#stubs-package')).toBeNull()
    const zip = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Download zip')) as HTMLButtonElement
    zip.click()
    await settle()
    expect(bodies().at(-1)).toMatchObject({ format: 'zip', query: 'execution_type:automated' })
    expect(bodies().some((b) => b.mark_script)).toBe(false)
    app.unmount()
  })

  it('marks the scripts of the selected tests', async () => {
    const { app, marked } = await mount({ ids: ['a', 'b'] })
    expect(bodies()[0].ids).toEqual(['a', 'b'])
    expect(document.querySelector('[data-testid="stubs-scope"]')?.textContent).toContain('2 selected tests')
    ;(document.querySelector('#stubs-mark') as HTMLInputElement).click()
    await settle()
    const zip = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Download zip')) as HTMLButtonElement
    zip.click()
    await settle()
    const sent = bodies()
    expect(sent.find((b) => b.mark_script)).toMatchObject({ mark_script: true, format: 'json', ids: ['a', 'b'] })
    expect(sent.at(-1)).toMatchObject({ format: 'zip' })
    expect(sent.at(-1).mark_script).toBeUndefined()
    expect(marked).toHaveBeenCalledWith(2)
    app.unmount()
  })

  it('hides marking without test.write and shows errors', async () => {
    fetchMock.mockImplementation(async () => new Response(JSON.stringify({ error: 'unknown field "bogus"', code: 'bad_request' }), { status: 400, headers: { 'Content-Type': 'application/json' } }))
    const { app } = await mount({ canWrite: false, query: 'bogus:x' })
    expect(document.querySelector('#stubs-mark')).toBeNull()
    expect(document.querySelector('.p-message')?.textContent).toContain('unknown field')
    app.unmount()
  })
})

describe('stubsCommand', () => {
  it('quotes for the shell', () => {
    expect(shellQuote('tests/')).toBe('tests/')
    expect(shellQuote("it's")).toBe(`'it'\\''s'`)
    expect(shellQuote('')).toBe("''")
    expect(stubsCommand({ framework: 'go', query: 'execution_type:automated has:!script', dir: 'e2e', pkg: 'e2e' })).toBe(
      "qtestframe stubs --framework go --query 'execution_type:automated has:!script' --out e2e --package e2e",
    )
    expect(stubsCommand({ framework: 'pytest', keys: ['A-TC-1', 'A-TC-2'], markScript: true })).toBe("qtestframe stubs --framework pytest --query '' --ids A-TC-1,A-TC-2 --out . --mark-script")
  })
})

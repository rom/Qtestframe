import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import { i18n } from '@/i18n'
import TestImportDialog from './TestImportDialog.vue'
import type { TestImportPreview, TestImportResult } from '@/lib/testImport'

const uploadMock = vi.fn()
vi.mock('@/lib/upload', () => ({ upload: (...a: unknown[]) => uploadMock(...a) }))
vi.mock('@/composables/impact', () => ({ impactGateFor: async () => ({ impact: 'suspect' }) }))

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const preview: TestImportPreview = {
  source: 'testrail', source_name: 'TestRail', format: 'csv', file: 'cases.csv', target_folder: '', total: 2,
  summary: { new: 1, update: 0, unchanged: 0, conflict: 1, error: 0 },
  rows: [
    { index: 0, location: 'row 2', external_id: 'C1', title: 'Sign in', folder: 'Accounts', type: 'Functional', priority: 'high', status: 'draft', steps: 3, action: 'new', links: ['TR-REQ-1'], unresolved: ['SHOP-9'], executions: 1, warnings: ['no requirement matches SHOP-9'], errors: [] },
    { index: 1, location: 'row 5', external_id: 'C2', title: 'Sign out', folder: '', type: '', priority: 'medium', status: 'draft', steps: 1, action: 'conflict', key: 'TR-TC-2', links: [], unresolved: [], executions: 0, warnings: [], errors: [] },
  ],
  priorities: [{ value: 'High', count: 1, target: 'high', guessed: true }],
  statuses: [],
  fields: [{ name: 'Component', count: 2, sample: 'Accounts', target: '', guessed: true }],
  custom_fields: [{ key: 'component', label: 'Component', type: 'text' }],
  priority_options: ['critical', 'high', 'medium', 'low'],
  status_options: ['draft', 'ready', 'review', 'deprecated'],
  history: { runs: 1, executions: 1, known: 0 },
  unresolved_links: ['SHOP-9'],
  previous: 1,
  warnings: [],
  errors: [],
}
const result: TestImportResult = {
  created: 1, updated: 0, unchanged: 0, skipped: 1, folders_created: 1, links_added: 1, runs_created: 1, results_imported: 1, unresolved_links: ['SHOP-9'],
  items: [{ index: 0, id: 't1', key: 'TR-TC-1', title: 'Sign in', action: 'created' }], skipped_rows: [{ index: 1, location: 'row 5', title: 'Sign out', reason: 'conflict' }], warnings: [],
}

async function settle() {
  for (let i = 0; i < 10; i++) {
    await Promise.resolve()
    await nextTick()
  }
}

afterEach(() => {
  uploadMock.mockReset()
  document.body.innerHTML = ''
})

async function mount() {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }, { path: '/help/:page', name: 'help', component: { render: () => null } }] })
  const visible = ref(false)
  const imported = vi.fn()
  const app = createApp({
    render: () => h(TestImportDialog, { projectId: 'p1', folders: [], visible: visible.value, 'onUpdate:visible': (v: boolean) => (visible.value = v), onImported: imported }),
  })
  app.use(pinia).use(router).use(PrimeVue, { theme: 'none' }).use(ToastService).use(ConfirmationService).use(i18n)
  app.mount(el)
  visible.value = true
  await settle()
  return { app, imported }
}

function pick(name: string) {
  const input = document.querySelector('input[type=file]') as HTMLInputElement
  Object.defineProperty(input, 'files', { value: [new File(['ID,Title\n'], name)], configurable: true })
  input.dispatchEvent(new Event('change'))
}

describe('TestImportDialog', () => {
  it('previews a file, maps a field and saves', async () => {
    uploadMock.mockImplementation(async (path: string) => (path.includes('/apply') ? result : preview))
    const { app, imported } = await mount()
    const radios = document.querySelectorAll('input[type=radio]')
    expect(radios.length).toBe(5)
    pick('cases.csv')
    await settle()
    expect(uploadMock).toHaveBeenCalledTimes(1)
    expect(uploadMock.mock.calls[0][0]).toBe('/projects/p1/tests/import/preview')
    expect(uploadMock.mock.calls[0][2].fields.source).toBe('testrail')
    const text = document.body.textContent ?? ''
    expect(text).toContain('SHOP-9')
    expect(text).toContain('Sign in')
    expect(text).toContain('Verifies TR-REQ-1')
    // the history checkbox and the conflict option are offered
    expect(document.getElementById('ti-history')).not.toBeNull()
    expect(document.getElementById('ti-overwrite')).not.toBeNull()
    const save = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Save'))
    expect(save).toBeTruthy()
    save!.click()
    await settle()
    const apply = uploadMock.mock.calls.find((c) => String(c[0]).includes('/apply?impact='))
    expect(apply).toBeTruthy()
    expect(apply![2].fields.history).toBe('true')
    expect(apply![2].fields.priorities).toBe('{"High":"high"}')
    expect(imported).toHaveBeenCalledWith(result)
    expect(document.body.textContent).toContain('1 created')
    app.unmount()
  })

  it('refuses a file the source does not export', async () => {
    const { app } = await mount()
    const azure = document.querySelector('input[type=radio][value=azure]') as HTMLInputElement | null
    ;(azure ?? document.querySelectorAll('input[type=radio]')[4] as HTMLInputElement).click()
    await settle()
    pick('suite.xml')
    await settle()
    expect(uploadMock).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('suite.xml')
    app.unmount()
  })

  it('shows a preview error', async () => {
    uploadMock.mockRejectedValue(new Error('no header row found'))
    const { app } = await mount()
    pick('cases.csv')
    await settle()
    expect(document.querySelector('[role=alert]')?.textContent).toContain('no header row found')
    app.unmount()
  })
})

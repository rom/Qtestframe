import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import { i18n } from '@/i18n'
import CISetupPanel from './CISetupPanel.vue'
import { useAuthStore } from '@/stores/auth'
import type { CISetup } from '@/lib/ci'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const setup: CISetup = {
  server_url: 'https://q.example',
  project_key: 'SHOP',
  project_id: 'p1',
  token_role: 'tester',
  variables: [
    { name: 'QTF_URL', value: 'https://q.example', masked: false, required: true, description: '' },
    { name: 'QTF_TOKEN', masked: true, required: true, description: '' },
    { name: 'QTF_RELEASE', masked: false, required: false, description: '' },
  ],
  templates: [
    { name: 'gitlab-ci.yml', kind: 'gitlab', file: '.gitlab-ci.yml', url: 'https://q.example/api/v1/ci/templates/gitlab-ci.yml', content: 'variables:\n  QTF_PROJECT: "SHOP"\n' },
    { name: 'github-actions.yml', kind: 'github', file: '.github/workflows/qtestframe.yml', content: 'name: tests\n' },
  ],
}

const fetchMock = vi.fn()

async function settle() {
  for (let i = 0; i < 8; i++) {
    await Promise.resolve()
    await nextTick()
  }
}

beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation(async () => new Response(JSON.stringify(setup), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => vi.unstubAllGlobals())

async function mount() {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }, { path: '/admin/tokens', name: 'tokens', component: { render: () => null } }] })
  const app = createApp({ render: () => h(CISetupPanel, { projectId: 'p1' }) })
  app.use(pinia).use(router).use(PrimeVue, { theme: 'none' }).use(ToastService).use(ConfirmationService).use(i18n)
  useAuthStore().set({ user: { id: 'u1', email: 'a@b', display_name: 'A', preferences: {} }, tenant: { id: 't1' }, role: 'admin', permissions: ['token.manage', 'project.read'] } as never)
  app.mount(el)
  await settle()
  return { el, app }
}

describe('CISetupPanel', () => {
  it('shows the variables, the steps and the templates', async () => {
    const { el, app } = await mount()
    expect(fetchMock.mock.calls[0][0]).toContain('/api/v1/projects/p1/ci/setup')
    expect(el.querySelector('#h-ci-setup')?.textContent).toBe('CI setup')
    const rows = el.querySelectorAll('.ci-vars tbody tr')
    expect(rows.length).toBe(3)
    expect(rows[0].textContent).toContain('https://q.example')
    expect(rows[1].textContent).toContain('Masked')
    expect(rows[2].textContent).toContain('Optional')
    expect(el.querySelector('.ci-steps')?.textContent).toContain('role Tester')
    expect(el.querySelector('.ci-steps a')?.getAttribute('href')).toBe('/admin/tokens')
    const pre = el.querySelector('pre.ci-template') as HTMLElement
    expect(pre.textContent).toContain('QTF_PROJECT: "SHOP"')
    expect(pre.getAttribute('aria-label')).toBe('Template .gitlab-ci.yml')
    expect(el.querySelector('.ci-include')?.textContent).toContain("remote: 'https://q.example/api/v1/ci/templates/gitlab-ci.yml'")
    // another template
    const buttons = el.querySelectorAll('.p-selectbutton .p-togglebutton')
    expect(buttons.length).toBe(2)
    ;(buttons[1] as HTMLElement).click()
    await settle()
    expect(el.querySelector('pre.ci-template')?.textContent).toContain('name: tests')
    expect(el.querySelector('.ci-include')).toBeNull()
    app.unmount()
  })

  it('shows a load error', async () => {
    fetchMock.mockImplementation(async () => new Response(JSON.stringify({ error: 'boom', code: 'internal' }), { status: 500, headers: { 'Content-Type': 'application/json' } }))
    const { el, app } = await mount()
    expect(el.querySelector('.p-message')?.textContent).toContain('boom')
    app.unmount()
  })
})

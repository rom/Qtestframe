import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import { i18n } from '@/i18n'
import CIPostingSettings from './CIPostingSettings.vue'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const json = (v: unknown) => new Response(JSON.stringify(v), { status: 200, headers: { 'Content-Type': 'application/json' } })
const fetchMock = vi.fn()
let settings: Record<string, unknown>

async function settle() {
  for (let i = 0; i < 8; i++) {
    await Promise.resolve()
    await nextTick()
  }
  await new Promise((r) => setTimeout(r, 30))
  await nextTick()
}

beforeEach(() => {
  settings = { has_secret: false, has_ci_token: false, ci: { provider: '', api_url: '', project: '', post_status: false, post_note: false, release_id: '', conditions: [] } }
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
    if (url.includes('/code-settings')) return json(settings)
    if (url.includes('/releases')) return json({ items: [{ id: 'r1', name: '1.0' }] })
    if (url.includes('/ci/deliveries'))
      return json({ items: [{ id: 'd1', commit_sha: '0123456789abcdef', merge_request: '5', status: 'failed', error: 'GitLab answered 401', outcome: '', created_at: '2026-11-01T10:00:00Z' }] })
    if (url.includes('/ci-settings')) {
      const body = JSON.parse(String(init?.body))
      settings = { ...settings, ci: body.ci, has_ci_token: settings.has_ci_token || !!body.ci_token }
      if (body.clear_ci_token) settings.has_ci_token = false
      return json(settings)
    }
    return new Response('{}', { status: 404 })
  })
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => vi.unstubAllGlobals())

async function mount() {
  const el = document.createElement('div')
  document.body.appendChild(el)
  setActivePinia(createPinia())
  const app = createApp({ render: () => h(CIPostingSettings, { projectId: 'p1' }) })
  app.use(PrimeVue, { theme: 'none' }).use(ToastService).use(ConfirmationService).use(i18n)
  app.mount(el)
  await settle()
  return { el, app }
}

const input = (el: HTMLElement, sel: string, v: string) => {
  const i = el.querySelector(sel) as HTMLInputElement
  i.value = v
  i.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('CIPostingSettings', () => {
  it('turns posting on, asks for the token and saves', async () => {
    const { el, app } = await mount()
    expect(el.querySelector('[role="status"]')?.textContent).toContain('Posting is off')
    expect(el.querySelector('.ci-deliveries')?.textContent).toContain('GitLab answered 401')
    ;(el.querySelector('#ci-enabled') as HTMLInputElement).click()
    await settle()
    ;(el.querySelector('form') as HTMLFormElement).dispatchEvent(new Event('submit', { cancelable: true }))
    await settle()
    expect(el.textContent).toContain('Enter the project id or path.')
    expect(el.textContent).toContain('Enter the access token.')
    expect(fetchMock.mock.calls.some((c) => String(c[0]).includes('/ci-settings'))).toBe(false)
    const fields = el.querySelectorAll('.form-grid input')
    input(el, '#ci-token', 'tok')
    ;(fields[1] as HTMLInputElement).value = 'group/app'
    fields[1].dispatchEvent(new Event('input', { bubbles: true }))
    await settle()
    ;(el.querySelector('form') as HTMLFormElement).dispatchEvent(new Event('submit', { cancelable: true }))
    await settle()
    const put = fetchMock.mock.calls.find((c) => String(c[0]).includes('/ci-settings'))
    expect(put).toBeTruthy()
    const body = JSON.parse(String(put![1].body))
    expect(body).toMatchObject({ ci: { provider: 'gitlab', project: 'group/app', post_status: true, post_note: true }, ci_token: 'tok' })
    expect(el.querySelector('[role="status"]')?.textContent).toContain('token saved')
    app.unmount()
  })
})

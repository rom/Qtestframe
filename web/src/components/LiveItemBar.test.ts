import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { i18n } from '@/i18n'
import LiveItemBar from './LiveItemBar.vue'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { setEventSourceFactory, useLiveStore } from '@/stores/live'

const fetchMock = vi.fn()

async function settle() {
  for (let i = 0; i < 8; i++) {
    await Promise.resolve()
    await nextTick()
  }
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation(async () => json({ others: [{ user_id: 'u2', name: 'Bea', state: 'editing' }], ttl_ms: 45000 }))
  vi.stubGlobal('fetch', fetchMock)
  setEventSourceFactory(() => ({ addEventListener() {}, close() {}, onerror: null }))
})
afterEach(() => {
  vi.unstubAllGlobals()
  setEventSourceFactory(null)
})

async function mount(props: { itemId: string | null; version: number | null; dirty: boolean }) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const pinia = createPinia()
  setActivePinia(pinia)
  const state = reactive({ ...props })
  const emitted: string[] = []
  const app = createApp({
    render: () =>
      h(LiveItemBar, {
        type: 'requirement',
        ...state,
        onReload: () => emitted.push('reload'),
        onCompare: (by: string) => emitted.push('compare:' + by),
      }),
  })
  app.use(pinia).use(PrimeVue, { theme: 'none' }).use(i18n)
  useMetaStore().meta.live_updates = true
  useAuthStore().set({ user: { id: 'me', email: 'me@example.com', display_name: 'Me', preferences: {} }, tenant: { id: 't1' }, role: 'admin', permissions: [] } as never)
  app.mount(el)
  await settle()
  return { el, app, state, emitted, live: useLiveStore() }
}

const msg = (data: unknown) => ({ data: JSON.stringify(data), lastEventId: '' }) as MessageEvent

describe('LiveItemBar', () => {
  it('shows who else is on the item and sends presence heartbeats', async () => {
    const { el, app, state } = await mount({ itemId: 'r1', version: 3, dirty: false })
    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/api/v1/presence')
    expect(JSON.parse(String(init.body))).toEqual({ entity_type: 'requirement', entity_id: 'r1', state: 'viewing' })
    expect(el.querySelector('[data-testid="live-presence"]')?.textContent).toContain('Bea is also editing this')
    // starting to edit is told at once
    state.dirty = true
    await settle()
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1].body)).state).toBe('editing')
    app.unmount()
    await settle()
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1].body)).state).toBe('left')
  })

  it('shows a banner for a newer version by someone else, with reload and compare', async () => {
    const { el, app, state, emitted, live } = await mount({ itemId: 'r1', version: 3, dirty: true })
    const announcer = () => el.querySelector('[aria-live="polite"]')?.textContent ?? ''
    expect(announcer()).toBe('')
    // own saves and older versions are no news
    live.handle('change', msg({ type: 'requirement', entity_id: 'r1', version: 4, actor_id: 'me', actor_name: 'Me' }))
    live.handle('change', msg({ type: 'requirement', entity_id: 'r1', version: 3, actor_id: 'u2', actor_name: 'Bea' }))
    live.handle('change', msg({ type: 'requirement', entity_id: 'other', version: 9, actor_id: 'u2', actor_name: 'Bea' }))
    await settle()
    expect(el.querySelector('[data-testid="live-banner"]')).toBeNull()
    live.handle('change', msg({ type: 'requirement', entity_id: 'r1', version: 4, actor_id: 'u2', actor_name: 'Bea', action: 'requirement.update' }))
    await settle()
    const banner = el.querySelector('[data-testid="live-banner"]') as HTMLElement
    expect(banner.textContent).toContain('Bea changed this item (version 4).')
    expect(banner.textContent).toContain('Your changes are not saved yet.')
    expect(announcer()).toContain('Bea changed this item (version 4).')
    const buttons = [...banner.querySelectorAll('button')]
    expect(buttons.map((b) => b.textContent?.trim())).toEqual(['Reload', 'Compare', 'Dismiss'])
    buttons[1].click()
    expect(emitted).toEqual(['compare:Bea'])
    buttons[0].click()
    await settle()
    expect(emitted).toEqual(['compare:Bea', 'reload'])
    expect(el.querySelector('[data-testid="live-banner"]')).toBeNull()
    // loading the newer version clears a banner that came meanwhile
    live.handle('change', msg({ type: 'requirement', entity_id: 'r1', version: 5, actor_id: 'u2', actor_name: 'Bea' }))
    await settle()
    expect(el.querySelector('[data-testid="live-banner"]')).not.toBeNull()
    state.version = 5
    await settle()
    expect(el.querySelector('[data-testid="live-banner"]')).toBeNull()
    // deleted: no reload, no compare
    live.handle('change', msg({ type: 'requirement', entity_id: 'r1', version: 5, actor_id: 'u2', actor_name: 'Bea', action: 'requirement.delete' }))
    await settle()
    const gone = el.querySelector('[data-testid="live-banner"]') as HTMLElement
    expect(gone.textContent).toContain('Bea deleted this item.')
    expect([...gone.querySelectorAll('button')].map((b) => b.textContent?.trim())).toEqual(['Dismiss'])
    ;(gone.querySelector('button') as HTMLElement).click()
    await settle()
    expect(el.querySelector('[data-testid="live-banner"]')).toBeNull()
    app.unmount()
  })

  it('renders nothing for a new item', async () => {
    const { el, app } = await mount({ itemId: null, version: null, dirty: false })
    expect(el.querySelector('.live-item-bar')).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    app.unmount()
  })
})

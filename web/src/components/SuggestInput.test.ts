import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { i18n } from '@/i18n'
import SuggestInput from './SuggestInput.vue'
import { useAuthStore } from '@/stores/auth'
import { clearSuggestCache, SUGGEST_DEBOUNCE_MS } from '@/lib/suggest'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const fetchMock = vi.fn()
const answer = (values: string[]) =>
  new Response(JSON.stringify({ items: values.map((value, i) => ({ value, source: i === 1 ? 'project' : 'history', uses: 1 })), total: values.length }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

async function settle() {
  for (let i = 0; i < 6; i++) {
    await Promise.resolve()
    await nextTick()
  }
}

function mount(props: Record<string, unknown>, prefs: Record<string, unknown> = {}) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const pinia = createPinia()
  setActivePinia(pinia)
  const value = ref((props.modelValue as string) ?? '')
  const keys: string[] = []
  const app = createApp({
    render: () =>
      h(SuggestInput, {
        ...props,
        modelValue: value.value,
        'onUpdate:modelValue': (v: string) => (value.value = v),
        // the caller's own key handling (e.g. a form's Enter): must not see a used key
        onKeydown: (e: KeyboardEvent) => keys.push(e.key),
      } as never),
  })
  app.use(pinia).use(PrimeVue, { theme: 'none' }).use(i18n)
  useAuthStore().set({ user: { id: 'u1', email: 'a@b', display_name: 'A', preferences: prefs }, tenant: { id: 't1' } } as never)
  app.mount(el)
  const input = el.querySelector('input, textarea') as HTMLInputElement
  const type = async (text: string) => {
    input.focus()
    input.value = text
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
  }
  const key = async (k: string, init: KeyboardEventInit = {}) => {
    const e = new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...init })
    input.dispatchEvent(e)
    await settle()
    return e
  }
  return { el, app, input, value, keys, type, key }
}

beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation(async () => answer(['Login works', 'Login page opens', 'Log out']))
  vi.stubGlobal('fetch', fetchMock)
  clearSuggestCache()
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('SuggestInput', () => {
  it('waits for a pause in typing, then lists the suggestions as a combobox', async () => {
    const m = mount({ field: 'requirement.title', projectId: 'p1' })
    expect(m.input.getAttribute('role')).toBe('combobox')
    expect(m.input.getAttribute('aria-autocomplete')).toBe('list')
    expect(m.input.getAttribute('aria-expanded')).toBe('false')
    expect(m.input.getAttribute('autocomplete')).toBe('off')
    await m.type('L')
    await m.type('Lo')
    await m.type('Log')
    await vi.advanceTimersByTimeAsync(SUGGEST_DEBOUNCE_MS - 50)
    expect(fetchMock).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(60)
    await settle()
    // one request for the last text only
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(new URL(fetchMock.mock.calls[0][0], 'http://x').searchParams.get('q')).toBe('Log')
    const list = m.el.querySelector('[role="listbox"]') as HTMLElement
    expect(list).not.toBeNull()
    expect(m.input.getAttribute('aria-expanded')).toBe('true')
    expect(m.input.getAttribute('aria-controls')).toBe(list.id)
    expect([...list.querySelectorAll('[role="option"]')].map((o) => o.textContent?.trim())).toEqual(['Login works', 'Login page opensUsed in this project', 'Log out'])
    // nothing is chosen yet: no active descendant, and the count is announced
    expect(m.input.hasAttribute('aria-activedescendant')).toBe(false)
    expect(m.el.querySelector('[aria-live="polite"]')?.textContent).toContain('3 suggestions')
    m.app.unmount()
  })

  it('moves with the arrow keys and takes a suggestion with Enter; Escape closes', async () => {
    const m = mount({ field: 'test.step.expected', projectId: 'p1' })
    await m.type('Log')
    await vi.advanceTimersByTimeAsync(SUGGEST_DEBOUNCE_MS + 10)
    await settle()
    // Enter with nothing chosen keeps its meaning (the caller sees it)
    await m.key('Enter')
    expect(m.keys).toContain('Enter')
    expect(m.value.value).toBe('Log')
    await m.key('ArrowDown')
    await m.key('ArrowDown')
    const opts = [...m.el.querySelectorAll('[role="option"]')]
    expect(m.input.getAttribute('aria-activedescendant')).toBe(opts[1].id)
    expect(opts[1].getAttribute('aria-selected')).toBe('true')
    await m.key('ArrowUp')
    expect(m.input.getAttribute('aria-activedescendant')).toBe(opts[0].id)
    m.keys.length = 0
    const e = await m.key('Enter')
    expect(e.defaultPrevented).toBe(true)
    expect(m.keys).not.toContain('Enter') // the form does not submit
    expect(m.value.value).toBe('Login works')
    expect(m.el.querySelector('[role="listbox"]')).toBeNull()
    expect(m.el.querySelector('[aria-live="polite"]')?.textContent).toContain('Inserted: Login works')

    // ArrowDown on a single-line field opens the list without typing more
    await m.key('ArrowDown')
    expect(m.el.querySelector('[role="listbox"]')).not.toBeNull()
    const esc = await m.key('Escape')
    expect(esc.defaultPrevented).toBe(true)
    expect(m.el.querySelector('[role="listbox"]')).toBeNull()
    expect(m.input.getAttribute('aria-expanded')).toBe('false')
    m.app.unmount()
  })

  it('takes the highlighted suggestion with Tab, and Tab without one leaves the field', async () => {
    const m = mount({ field: 'comment', projectId: 'p1' })
    await m.type('Lo')
    await vi.advanceTimersByTimeAsync(SUGGEST_DEBOUNCE_MS + 10)
    await settle()
    const plain = await m.key('Tab')
    expect(plain.defaultPrevented).toBe(false)
    expect(m.el.querySelector('[role="listbox"]')).toBeNull()
    await m.key('ArrowDown') // reopen
    await m.key('ArrowUp') // to the last one
    const tab = await m.key('Tab')
    expect(tab.defaultPrevented).toBe(true)
    expect(m.value.value).toBe('Log out')
    m.app.unmount()
  })

  it('in a textarea keeps the textbox role and opens with Alt+ArrowDown only', async () => {
    const m = mount({ field: 'test.step.action', projectId: 'p1', multiline: true })
    expect(m.input.tagName).toBe('TEXTAREA')
    expect(m.input.hasAttribute('role')).toBe(false)
    expect(m.input.getAttribute('aria-autocomplete')).toBe('list')
    m.input.focus()
    await m.key('ArrowDown')
    expect(fetchMock).not.toHaveBeenCalled()
    await m.key('ArrowDown', { altKey: true })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(m.el.querySelectorAll('[role="option"]').length).toBe(3)
    // a click (mousedown) chooses
    m.el.querySelectorAll('[role="option"]')[2].dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true }))
    await settle()
    expect(m.value.value).toBe('Log out')
    m.app.unmount()
  })

  it('makes no request and adds no combobox when the preference is off', async () => {
    const m = mount({ field: 'requirement.title', projectId: 'p1' }, { autocomplete: 'off' })
    expect(m.input.hasAttribute('role')).toBe(false)
    expect(m.input.hasAttribute('autocomplete')).toBe(false)
    await m.type('Log')
    await vi.advanceTimersByTimeAsync(1000)
    await m.key('ArrowDown')
    await settle()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(m.el.querySelector('[role="listbox"]')).toBeNull()
    m.app.unmount()
  })

  it('does nothing without a project, while disabled, or when the field lost focus', async () => {
    const a = mount({ field: 'requirement.title' })
    await a.type('Log')
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchMock).not.toHaveBeenCalled()
    a.app.unmount()
    const b = mount({ field: 'requirement.title', projectId: 'p1', disabled: true })
    await b.type('Log')
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchMock).not.toHaveBeenCalled()
    b.app.unmount()
    const c = mount({ field: 'requirement.title', projectId: 'p1' })
    await c.type('Log')
    c.input.blur()
    await vi.advanceTimersByTimeAsync(1000)
    await settle()
    expect(c.el.querySelector('[role="listbox"]')).toBeNull()
    c.app.unmount()
  })

  it('closes on an empty answer or an error', async () => {
    fetchMock.mockImplementation(async () => answer([]))
    const m = mount({ field: 'risk.title', projectId: 'p1' })
    await m.type('zz')
    await vi.advanceTimersByTimeAsync(SUGGEST_DEBOUNCE_MS + 10)
    await settle()
    expect(m.el.querySelector('[role="listbox"]')).toBeNull()
    fetchMock.mockImplementation(async () => new Response('{"error":"x"}', { status: 500, headers: { 'Content-Type': 'application/json' } }))
    await m.type('zzz')
    await vi.advanceTimersByTimeAsync(SUGGEST_DEBOUNCE_MS + 10)
    await settle()
    expect(m.el.querySelector('[role="listbox"]')).toBeNull()
    m.app.unmount()
  })
})

import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, type Component } from 'vue'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, setLanguage } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import type { Me } from '@/lib/types'
import ObligationText from './ObligationText.vue'
import ObligationKeywordLine from './ObligationKeywordLine.vue'
import MentionText from './MentionText.vue'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

let pinia: Pinia
let el: HTMLElement
let unmount: (() => void) | null = null

async function mount(comp: Component, props: Record<string, unknown>) {
  el = document.createElement('div')
  document.body.appendChild(el)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }, { path: '/go/:key', name: 'go', component: { render: () => null } }] })
  const app = createApp({ render: () => h(comp, props) })
  app.use(pinia).use(i18n).use(router)
  app.mount(el)
  unmount = () => app.unmount()
  for (let i = 0; i < 3; i++) await nextTick()
  return el
}
const marks = (root: HTMLElement) => [...root.querySelectorAll('.obligation-kw')]
const markText = (m: Element) => m.firstChild?.textContent

function signIn(prefs: Record<string, unknown> = {}) {
  useAuthStore().set({ user: { id: 'u1', email: 'a@example.com', preferences: prefs } } as unknown as Me)
}

beforeEach(() => {
  pinia = createPinia()
  setActivePinia(pinia)
  setLanguage('en')
})
afterEach(() => {
  unmount?.()
  el?.remove()
})

describe('ObligationText', () => {
  it('marks the keywords with a level class, a title and screen-reader text', async () => {
    signIn()
    const root = await mount(ObligationText, { text: 'The system MUST NOT log it and SHOULD warn; MAYBE later.' })
    const ms = marks(root)
    expect(ms.map(markText)).toEqual(['MUST NOT', 'SHOULD'])
    expect(ms[0].className).toBe('obligation-kw obligation-must-not')
    expect(ms[1].className).toBe('obligation-kw obligation-should')
    expect(ms[0].getAttribute('title')).toContain('MUST NOT')
    expect(ms[0].querySelector('.sr-only')?.textContent?.trim()).toBe('(obligation: Must not)')
    // the visible text is intact
    expect(root.textContent?.replace(/ \(obligation: [^)]+\)/g, '')).toBe('The system MUST NOT log it and SHOULD warn; MAYBE later.')
  })
  it('translates the hints', async () => {
    signIn()
    setLanguage('sv')
    const root = await mount(ObligationText, { text: 'Systemet SKA INTE spara det och FÅR logga.' })
    const ms = marks(root)
    expect(ms.map(markText)).toEqual(['SKA INTE', 'FÅR'])
    expect(ms[0].querySelector('.sr-only')?.textContent?.trim()).toMatch(/^– kravnivå: /)
  })
  it('never renders the text as HTML (XSS)', async () => {
    signIn()
    const text = '<img src=x onerror="window.__xss=1"> MUST <script>window.__xss=2</script><b>SHOULD</b>'
    const root = await mount(ObligationText, { text })
    expect(root.querySelector('img, script, b')).toBeNull()
    expect((window as unknown as { __xss?: number }).__xss).toBeUndefined()
    expect(marks(root).map(markText)).toEqual(['MUST', 'SHOULD'])
    expect(root.textContent).toContain('<img src=x onerror="window.__xss=1">')
  })
  it('does not mark inside code spans', async () => {
    signIn()
    const root = await mount(ObligationText, { text: 'Use `MUST` literally, but it MUST log.' })
    expect(marks(root).map(markText)).toEqual(['MUST'])
    expect(root.textContent).toContain('`MUST`')
  })
  it('follows the preference', async () => {
    signIn({ hide_obligation_highlight: true })
    const root = await mount(ObligationText, { text: 'The system MUST log.' })
    expect(marks(root)).toEqual([])
    expect(root.textContent).toBe('The system MUST log.')
  })
})

describe('MentionText with obligations', () => {
  it('marks keywords in the text but not inside links', async () => {
    signIn()
    const root = await mount(MentionText, { text: 'See https://example.test/MUST and SHOP-REQ-1: it MUST log.', obligations: true })
    expect(marks(root).map(markText)).toEqual(['MUST'])
    expect(root.querySelector('a.text-link')?.textContent).toContain('https://example.test/MUST')
    expect(root.querySelector('a.text-link .obligation-kw')).toBeNull()
  })
  it('marks nothing without the prop', async () => {
    signIn()
    const root = await mount(MentionText, { text: 'It MUST log.' })
    expect(marks(root)).toEqual([])
  })
})

describe('ObligationKeywordLine', () => {
  it('shows the keyword and flags a mismatch, several and lower-case keywords', async () => {
    signIn()
    const applied: string[] = []
    const root = await mount(ObligationKeywordLine, { text: 'The system SHOULD log and must alert.', chosen: 'may', canWrite: true, onApply: (l: string) => applied.push(l) })
    expect(root.textContent).toContain('SHOULD')
    expect(root.textContent).toContain('but the obligation field is May')
    expect(root.textContent).toContain('Several obligation keywords (SHOULD, MUST)')
    expect(root.textContent).toContain('In lower case: must')
    const apply = [...root.querySelectorAll('button')].find((b) => b.textContent?.includes('Set the obligation'))
    apply?.click()
    expect(applied).toEqual(['must'])
  })
  it('confirms a match and suggests keywords when there is none', async () => {
    signIn()
    let root = await mount(ObligationKeywordLine, { text: 'The system MUST log.', chosen: 'must' })
    expect(root.textContent).toContain('Matches the obligation field.')
    unmount?.()
    root = await mount(ObligationKeywordLine, { text: 'The system logs.', chosen: '' })
    expect(root.textContent).toContain('No obligation keyword yet')
    expect(root.textContent).toContain('MUST, SHALL')
  })
})

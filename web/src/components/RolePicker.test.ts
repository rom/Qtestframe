import { describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import PrimeVue from 'primevue/config'
import { i18n, setLanguage } from '@/i18n'
import RolePicker from './RolePicker.vue'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

async function mount(initial: string[], props: Record<string, unknown> = {}) {
  setLanguage('en')
  const el = document.createElement('div')
  document.body.appendChild(el)
  const roles = ref(initial)
  const changes: string[][] = []
  const app = createApp({
    render: () =>
      h(RolePicker, {
        modelValue: roles.value,
        'onUpdate:modelValue': (v: string[]) => (roles.value = v),
        onChange: (v: string[]) => changes.push(v),
        options: ['admin', 'tester', 'sme', 'viewer'],
        ariaLabel: 'Roles of sara@example.com',
        ...props,
      }),
  })
  app.use(PrimeVue, { theme: 'none' }).use(i18n)
  app.directive('tooltip', {})
  app.mount(el)
  await nextTick()
  return { el, roles, changes, unmount: () => app.unmount() }
}

const chips = (el: HTMLElement) => [...el.querySelectorAll('.role-chip')].map((c) => c.textContent?.trim())

describe('RolePicker', () => {
  it('shows the roles as chips in role order in a labelled group', async () => {
    const { el, unmount } = await mount(['sme', 'tester'])
    expect(chips(el)).toEqual(['Tester', 'Subject matter expert'])
    expect(el.querySelector('[role="group"]')?.getAttribute('aria-label')).toBe('Roles of sara@example.com')
    expect(el.querySelectorAll('.chip-remove')).toHaveLength(2)
    expect(el.querySelector('.chip-remove')?.getAttribute('aria-label')).toBe('Remove the role Tester')
    unmount()
  })
  it('removes a role but never the last one', async () => {
    const { el, roles, changes, unmount } = await mount(['sme', 'tester'])
    ;(el.querySelector('.chip-remove') as HTMLButtonElement).click()
    await nextTick()
    expect(roles.value).toEqual(['sme'])
    expect(changes).toEqual([['sme']])
    expect(chips(el)).toEqual(['Subject matter expert'])
    expect(el.querySelectorAll('.chip-remove')).toHaveLength(0)
    unmount()
  })
  it('locks roles the caller may not give and hides adding when disabled', async () => {
    const a = await mount(['admin', 'sme'], { locked: ['admin'] })
    expect(a.el.querySelectorAll('.chip-remove')).toHaveLength(1)
    expect(a.el.querySelector('.role-add')).not.toBeNull()
    a.unmount()
    const b = await mount(['sme', 'tester'], { disabled: true })
    expect(b.el.querySelectorAll('.chip-remove')).toHaveLength(0)
    expect(b.el.querySelector('.role-add')).toBeNull()
    b.unmount()
    // every offered role held: nothing to add
    const c = await mount(['admin', 'tester', 'sme', 'viewer'])
    expect(c.el.querySelector('.role-add')).toBeNull()
    c.unmount()
  })
})

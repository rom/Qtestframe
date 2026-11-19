import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ThemePicker from '@/components/ThemePicker.vue'
import { usePrefsStore } from '@/stores/prefs'
import { useAuthStore } from '@/stores/auth'
import { resetThemeState } from './apply'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const flush = async () => {
  for (let i = 0; i < 5; i++) await nextTick()
  await new Promise((r) => setTimeout(r, 20))
}

function mount(props: Record<string, unknown> = {}, onUpdate?: (v: string) => void) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const pinia = createPinia()
  setActivePinia(pinia)
  const value = ref(props.modelValue as string | undefined)
  const app = createApp({
    render: () =>
      h(ThemePicker, {
        ...props,
        modelValue: value.value,
        'onUpdate:modelValue': (v: string) => {
          value.value = v
          onUpdate?.(v)
        },
      } as never),
  })
  app.use(pinia).use(PrimeVue, { theme: 'none' })
  app.mount(el)
  return { el, app, value }
}

describe('ThemePicker', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    resetThemeState()
  })

  it('shows the stored preference with an accessible name', async () => {
    const { el } = mount()
    const auth = useAuthStore()
    auth.set({ user: { id: 'u', email: 'a@b', display_name: 'A', preferences: { theme: 'navy' } } } as never)
    await nextTick()
    expect(el.textContent).toContain('Navy')
    expect(el.querySelector('[aria-label="Colour theme"]')).not.toBeNull()
  })

  it('uses labelFor, then t, then English', async () => {
    const a = mount({ modelValue: 'crazy', labelFor: (t: string) => `L:${t}` })
    expect(a.el.textContent).toContain('L:crazy')
    a.app.unmount()
    const b = mount({ modelValue: 'spring', t: (k: string) => (k === 'theme.spring' ? 'Frühling' : k), ariaLabel: undefined })
    expect(b.el.textContent).toContain('Frühling')
    expect(b.el.querySelector('[aria-label="Colour theme"]')).not.toBeNull()
    b.app.unmount()
    const c = mount({ modelValue: '', allowUnset: true, t: (k: string) => (k === 'theme.picker' ? 'Farbschema' : k) })
    expect(c.el.textContent).toContain('Not set (this device)')
    expect(c.el.querySelector('[aria-label="Farbschema"]')).not.toBeNull()
  })

  it('emits and saves the choice', async () => {
    const onUpdate = vi.fn()
    const { el } = mount({ modelValue: 'light' }, onUpdate)
    const prefs = usePrefsStore()
    const setTheme = vi.spyOn(prefs, 'setTheme').mockResolvedValue()
    ;(el.querySelector('.p-select') as HTMLElement).click()
    await flush()
    const option = [...document.querySelectorAll('[role="option"]')].find((o) => o.textContent?.includes('Green console')) as HTMLElement
    expect(option).toBeTruthy()
    expect(document.querySelectorAll('[role="option"]')).toHaveLength(11)
    option.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await flush()
    expect(onUpdate).toHaveBeenCalledWith('green_console')
    expect(setTheme).toHaveBeenCalledWith('green_console')
  })
})

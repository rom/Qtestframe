import { describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import PrimeVue from 'primevue/config'
import { i18n } from '@/i18n'
import SystemSettingField from './SystemSettingField.vue'
import type { FormValue, SettingView } from '@/lib/systemSettings'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const flush = async () => {
  for (let i = 0; i < 5; i++) await nextTick()
  await new Promise((r) => setTimeout(r, 20))
}

function setting(p: Partial<SettingView>): SettingView {
  return { key: 'logging.level', section: 'logging', type: 'enum', apply: 'runtime', options: ['debug', 'info'], value: 'info', pending_restart: false, file_value: 'info', file_source: 'default', source: 'default', stored: false, ignored: false, locked: false, ...p }
}

function mount(s: SettingView, initial: FormValue) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const value = ref<FormValue>(initial)
  const remove = ref(false)
  const resets: number[] = []
  const app = createApp({
    render: () =>
      h(SystemSettingField, {
        setting: s,
        modelValue: value.value,
        removeSecret: remove.value,
        'onUpdate:modelValue': (v: FormValue) => (value.value = v),
        'onUpdate:removeSecret': (v: boolean) => (remove.value = v),
        onReset: () => resets.push(1),
      }),
  })
  app.use(PrimeVue, { theme: 'none' }).use(i18n)
  app.mount(el)
  return { el, app, value, remove, resets }
}

describe('SystemSettingField', () => {
  it('shows an enum in a select labelled by the setting', async () => {
    const m = mount(setting({}), 'info')
    await flush()
    expect(m.el.querySelector('.p-select-label')?.textContent?.trim()).toBe('info')
    const label = m.el.querySelector('label.sys-label') as HTMLLabelElement
    expect(label.textContent).toBe('Log level')
    expect(m.el.querySelector(`#${label.htmlFor}`)).not.toBeNull()
    m.app.unmount()
  })
  it('updates text and shows the source, lock and file value', async () => {
    const m = mount(setting({ key: 'email.host', type: 'string', value: 'a', source: 'database', stored: true, file_value: '', updated_by: 'x@example.com', updated_at: '2026-01-01T00:00:00Z' }), 'a')
    await flush()
    const input = m.el.querySelector('input') as HTMLInputElement
    input.value = 'b'
    input.dispatchEvent(new Event('input'))
    await flush()
    expect(m.value.value).toBe('b')
    expect(m.el.textContent).toContain('Changed here')
    expect(m.el.textContent).toContain('File value: (empty)')
    ;([...m.el.querySelectorAll('button')].find((b) => b.textContent?.includes('Reset')) as HTMLElement).click()
    expect(m.resets.length).toBe(1)
    m.app.unmount()
    const l = mount(setting({ key: 'logging.level', locked: true, lock_reason: 'environment', env: 'QTF_LOG_LEVEL', source: 'environment' }), 'info')
    await flush()
    expect(l.el.textContent).toContain('QTF_LOG_LEVEL')
    l.app.unmount()
  })
  it('handles secrets, booleans, numbers and restart settings', async () => {
    const s = mount(setting({ key: 'email.password', type: 'secret', value: null, has_secret: true, file_has_secret: false, source: 'database', stored: true }), '')
    await flush()
    expect(s.el.textContent).toContain('A value is set')
    ;(s.el.querySelector('.p-checkbox input') as HTMLInputElement).click()
    await flush()
    expect(s.remove.value).toBe(true)
    s.app.unmount()
    const b = mount(setting({ key: 'runner.enabled', type: 'bool', apply: 'restart', value: true, running_value: false, pending_restart: true }), true)
    await flush()
    expect(b.el.textContent).toContain('Applies after restart')
    expect(b.el.textContent).toContain('In use until the restart: Off')
    b.app.unmount()
    const n = mount(setting({ key: 'email.port', type: 'int', value: 25, min: 1, max: 65535 }), 25)
    await flush()
    expect((n.el.querySelector('input') as HTMLInputElement).value).toBe('25')
    n.app.unmount()
    const r = mount(setting({ key: 'server.trusted_proxies', type: 'list', value: ['10.0.0.0/8'], locked: true, lock_reason: 'readonly' }), '10.0.0.0/8')
    await flush()
    expect(r.el.textContent).toContain('Configuration file only')
    r.app.unmount()
  })
})

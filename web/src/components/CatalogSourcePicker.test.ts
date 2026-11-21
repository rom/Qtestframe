import { describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import PrimeVue from 'primevue/config'
import { i18n } from '@/i18n'
import CatalogSourcePicker from './CatalogSourcePicker.vue'
import { complianceClass } from '@/lib/compliance'
import type { CatalogSource } from '@/lib/catalog'

window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })) as never

const flush = async () => {
  for (let i = 0; i < 5; i++) await nextTick()
  await new Promise((r) => setTimeout(r, 20))
}

const src = (id: string, kind: string, name: string, n: number): CatalogSource => ({ id, kind, name, requirements: n, tests: n + 1, catalogues: ['software'], jurisdiction: 'EU' })
const sources = [src('gdpr', 'law', 'General Data Protection Regulation', 40), src('cra', 'law', 'Cyber Resilience Act', 30), src('iso-27001', 'standard', 'ISO/IEC 27001', 70)]
const kinds = [
  { id: 'law', name: 'Laws and regulations' },
  { id: 'standard', name: 'Standards' },
]

describe('CatalogSourcePicker', () => {
  it('groups the sources by kind, counts and applies the choice in registry order', async () => {
    const el = document.createElement('div')
    document.body.appendChild(el)
    const visible = ref(false)
    const applied: string[][] = []
    const app = createApp({
      render: () =>
        h(CatalogSourcePicker, {
          visible: visible.value,
          'onUpdate:visible': (v: boolean) => (visible.value = v),
          sources,
          kinds,
          selected: ['cra'],
          onApply: (ids: string[]) => applied.push(ids),
        }),
    })
    app.use(PrimeVue, { theme: 'none' }).use(i18n)
    app.mount(el)
    visible.value = true
    await flush()
    const legends = [...document.body.querySelectorAll('fieldset legend')].map((x) => x.textContent?.trim())
    expect(legends).toEqual(['Laws and regulations (2)', 'Standards (1)'])
    const cra = document.body.querySelector('#src-cra') as HTMLInputElement
    expect(cra.checked).toBe(true)
    const label = document.body.querySelector('label[for="src-gdpr"]')
    expect(label?.textContent).toContain('40 requirements · 41 tests')
    // choose GDPR too, then apply
    ;(document.body.querySelector('#src-gdpr') as HTMLInputElement).click()
    await flush()
    expect(document.body.textContent).toContain('2 sources chosen: 70 requirements and 72 tests')
    const apply = [...document.body.querySelectorAll('button')].find((b) => b.textContent?.includes('Show the requirements')) as HTMLButtonElement
    apply.click()
    await flush()
    expect(applied).toEqual([['gdpr', 'cra']])
    expect(visible.value).toBe(false)
    app.unmount()
    el.remove()
  })
  it('colours compliance percentages', () => {
    expect([complianceClass(85), complianceClass(50), complianceClass(10)]).toEqual(['good', 'fair', 'poor'])
  })
})
